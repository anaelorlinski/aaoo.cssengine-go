package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
)

// Compound is a compound CSS chunk, like a declaration
// or a qualified rule.
type Compound interface {
	Pos() Pos
	isCompound()
}

type QualifiedRule struct {
	Prelude, Content []Token
	pos              Pos
}

type AtRule struct {
	AtKeyword string
	QualifiedRule
}

type Declaration struct {
	Name      string
	Value     []Token
	pos       Pos
	Important bool
}

func (QualifiedRule) isCompound() {}
func (AtRule) isCompound()        {}
func (Declaration) isCompound()   {}
func (ParseError) isCompound()    {}
func (Whitespace) isCompound()    {}
func (Comment) isCompound()       {}

func (t QualifiedRule) Pos() Pos { return t.pos }
func (t AtRule) Pos() Pos        { return t.pos }
func (t Declaration) Pos() Pos   { return t.pos }

// Already defined:
// 	ParseError
// 	Whitespace
// 	Comment

// ParseOneDeclaration parses a single `declaration`, returning a [ParseError] or a [Declaration]
//
// This is used e.g. for a declaration in an `@supports
// <http://drafts.csswg.org/csswg/css-conditional/#at-supports>.
// Any whitespace or comment before the “:“ colon is dropped.
func ParseOneDeclaration(input []Token) Compound {
	tokens := NewIter(input)
	firstToken := tokens.NextSignificant()
	if firstToken == nil {
		return ParseError{pos: Pos{1, 1}, kind: errEmpty, Message: "Input is empty"}
	}
	return parseDeclaration(firstToken, tokens, false)
}

func consumeRemnants(input *TokensIter, nested bool) {
	for input.HasNext() {
		token := input.Next()
		if IsLiteral(token, ";") {
			return
		}
		if nested && IsLiteral(token, "}") {
			return
		}
	}
}

// parses a declaration, by consuming `tokens`
// until the end of the declaration or the first error.
// returns either a [ParseError] or a [Declaration]
func parseDeclaration(firstToken Token, tokens *TokensIter, nested bool) Compound {
	name, ok := firstToken.(Ident)
	if !ok {
		consumeRemnants(tokens, nested)
		return ParseError{
			pos:     firstToken.Pos(),
			kind:    errInvalid,
			Message: fmt.Sprintf("Expected <ident> for declaration name, got %s.", firstToken.Kind()),
		}
	}
	colon := tokens.NextSignificant()
	if colon == nil {
		consumeRemnants(tokens, nested)
		return ParseError{
			pos:     firstToken.Pos(),
			kind:    errInvalid,
			Message: "Expected ':' after declaration name, got EOF",
		}
	} else if !IsLiteral(colon, ":") {
		return ParseError{
			pos:     colon.Pos(),
			kind:    errInvalid,
			Message: fmt.Sprintf("Expected ':' after declaration name, got %s.", colon.Kind()),
		}
	}

	var (
		value                    []Token
		blocks, otherSignificant int // {} blocks, other significant tokens
	)
	for tokens.HasNext() {
		token := tokens.Next()
		switch token.Kind() {
		case KWhitespace, KComment:
		case KCurlyBracketsBlock:
			blocks++
		default:
			otherSignificant++
		}
		value = append(value, token)
	}

	// !important: the last two significant tokens (CSS Syntax 3 §5.4.6)
	important := false
	if last := lastSignificant(value, len(value)); last >= 0 {
		if ident, ok := value[last].(Ident); ok && utils.AsciiLower(ident.Value) == "important" {
			if bang := lastSignificant(value, last); bang >= 0 && IsLiteral(value[bang], "!") {
				important = true
				value = value[:bang]
			}
		}
	}

	// a {} block with other values is only valid in a custom property
	// (!important does not count)
	if important {
		otherSignificant -= 2
	}
	if blocks > 0 && blocks+otherSignificant > 1 && !strings.HasPrefix(name.Value, "--") {
		return ParseError{pos: colon.Pos(), kind: errInvalid, Message: "Declaration contains {} block"}
	}

	return Declaration{
		pos:       name.pos,
		Name:      name.Value,
		Value:     value,
		Important: important,
	}
}

// lastSignificant returns the index of the last token of tokens[:end]
// which is neither whitespace nor a comment, or -1.
func lastSignificant(tokens []Token, end int) int {
	for i := end - 1; i >= 0; i-- {
		switch tokens[i].Kind() {
		case KWhitespace, KComment:
		default:
			return i
		}
	}
	return -1
}

// Like `parseDeclaration`, but stop at the first “;“.
func consumeDeclarationInList(firstToken Token, tokens *TokensIter) Compound {
	var otherDeclarationTokens []Token
	for tokens.HasNext() {
		token := tokens.Next()
		if IsLiteral(token, ";") {
			break
		}
		otherDeclarationTokens = append(otherDeclarationTokens, token)
	}
	return parseDeclaration(firstToken, NewIter(otherDeclarationTokens), false)
}

// ParseBlocksContents parses a block’s contents.
//
// This is used e.g. for the [QualifiedRule] content
// of a style rule or '@page' rule, or for the 'style' attribute of an
// HTML element.
//
// In contexts that don’t expect any at-rule and/or qualified rule,
// all [AtRule] and [QualifiedRule] objects should simply be rejected as
// invalid.
func ParseBlocksContents(input []Token, skipWhitespace bool) []Compound {
	tokens := NewIter(input)
	var result []Compound
	for tokens.HasNext() {
		token := tokens.Next()
		switch token := token.(type) {
		case Whitespace:
			if !skipWhitespace {
				result = append(result, token)
			}
		case Comment:
			result = append(result, token)
		case AtKeyword:
			result = append(result, consumeAtRule(token, tokens))
		case Literal:
			if token.Value != ";" {
				result = append(result, consumeBlocksContent(token, tokens))
			}
		default:
			result = append(result, consumeBlocksContent(token, tokens))
		}
	}
	return result
}

// ParseBlocksContentsString tokenizes `css` and calls `ParseBlocksContents`.
func ParseBlocksContentsString(css string) []Compound {
	l := Tokenize([]byte(css), false)
	return ParseBlocksContents(l, false)
}

// ParseDeclarationList parses a `declaration list` (which may also contain at-rules).
// This is used e.g. for the `QualifiedRule.content`
// of a style rule or “@page“ rule, or for the “style“ attribute of an HTML element.
//
// In contexts that don’t expect any at-rule, all `AtRule` objects should simply be rejected as invalid.
//
// If `skipComments`, ignore CSS comments at the top-level of the list.
// If `skipWhitespace`, ignore whitespace at the top-level of the list. Whitespace is still preserved in
// the `Declaration.value` of declarations and the `AtRule.prelude` and `AtRule.content` of at-rules.
func ParseDeclarationList(input []Token, skipComments, skipWhitespace bool) []Compound {
	tokens := NewIter(input)
	var result []Compound

	for tokens.HasNext() {
		token := tokens.Next()
		switch token := token.(type) {
		case Whitespace:
			if !skipWhitespace {
				result = append(result, token)
			}
		case Comment:
			if !skipComments {
				result = append(result, token)
			}
		case AtKeyword:
			val := consumeAtRule(token, tokens)
			result = append(result, val)
		case Literal:
			if token.Value != ";" {
				result = append(result, consumeDeclarationInList(token, tokens))
			}
		default:
			result = append(result, consumeDeclarationInList(token, tokens))
		}
	}
	return result
}

// ParseDeclarationListString tokenizes `css` and calls `ParseDeclarationList`.
func ParseDeclarationListString(css string, skipComments, skipWhitespace bool) []Compound {
	l := Tokenize([]byte(css), skipComments)
	return ParseDeclarationList(l, skipComments, skipWhitespace)
}

// IsLiteral returns true if token is a literal [char].
func IsLiteral(token Token, char string) bool {
	lit, ok := token.(Literal)
	return ok && lit.Value == char
}

// Parse an at-rule, by consuming just enough of `tokens` for this rule.
// [atKeyword] is the token starting this rule.
func consumeAtRule(atKeyword AtKeyword, tokens *TokensIter) AtRule {
	var (
		prelude []Token
		content []Token
	)
	for tokens.HasNext() {
		token := tokens.Next()
		if curly, ok := token.(CurlyBracketsBlock); ok {
			content = curly.Arguments
			if content == nil {
				content = []Token{}
			}
			break
		}
		if IsLiteral(token, ";") {
			break
		}
		prelude = append(prelude, token)
	}
	return AtRule{
		AtKeyword: atKeyword.Value,
		QualifiedRule: QualifiedRule{
			pos:     atKeyword.pos,
			Prelude: prelude,
			Content: content,
		},
	}
}

// Parse a qualified rule or at-rule, by
// consuming just enough of `tokens` for this rule.
func consumeRule(firstToken Token, tokens *TokensIter) Compound {
	if atKeyword, isAtKeyword := firstToken.(AtKeyword); isAtKeyword {
		return consumeAtRule(atKeyword, tokens)
	}
	return consumeQualifiedRule(firstToken, tokens, false, false)
}

// Create rule parse error raised because of given token.
func ruleError(token Token, name string) ParseError {
	return ParseError{
		pos:     token.Pos(),
		kind:    errInvalid,
		Message: fmt.Sprintf("%s reached before {} block for a qualified rule.", name),
	}
}

// Consume a qualified rule.
// Consume just enough of :obj:`tokens` for this rule.
func consumeQualifiedRule(firstToken Token, tokens *TokensIter, nested bool, stopAtSemicolon bool) Compound {
	if stopAtSemicolon && IsLiteral(firstToken, ";") {
		return ruleError(firstToken, "Stop token")
	}

	var (
		prelude []Token
		block   CurlyBracketsBlock
	)
	if curly, isCurly := firstToken.(CurlyBracketsBlock); isCurly {
		block = curly
	} else {
		prelude = []Token{firstToken}
		hasBroken := false
		for tokens.HasNext() {
			token := tokens.Next()

			if stopAtSemicolon && IsLiteral(token, ";") {
				return ruleError(token, "Stop token")
			}

			if curly, ok := token.(CurlyBracketsBlock); ok {
				block = curly
				// TODO: handle special case for CSS variables (using "nested")
				// https://drafts.csswg.org/css-syntax-3/#consume-qualified-rule
				hasBroken = true
				break
			}
			prelude = append(prelude, token)
		}
		if !hasBroken {
			return ruleError(prelude[len(prelude)-1], "EOF")
		}
		// CSS Syntax 3 §5.4.3: a prelude starting like a custom property
		// declaration ("--x:") is not a rule
		if !nested && startsLikeCustomProperty(prelude) {
			return ParseError{pos: firstToken.Pos(), kind: errInvalid, Message: "A custom property declaration is not a rule."}
		}
	}

	return QualifiedRule{
		pos:     firstToken.Pos(),
		Content: block.Arguments,
		Prelude: prelude,
	}
}

// Consume declaration or nested rule.
func consumeBlocksContent(firstToken Token, tokens *TokensIter) Compound {
	// CSS Syntax 3 §5.4.4: a declaration runs to the next ";", {} blocks
	// included (a custom property may hold them); when it is not valid,
	// the input is read again as a nested rule, whose prelude ends at the
	// first {} block.
	//
	// The scan stops as soon as the result is known, keeping the parse
	// linear: at the first {} block when the tokens cannot start a
	// declaration (no "<ident> :") or make an invalid one (a block with
	// other values, outside a custom property).
	var declarationTokens, semicolonToken []Token
	start, firstBlock := tokens.index, -1
	if _, isCurly := firstToken.(CurlyBracketsBlock); !IsLiteral(firstToken, ";") && !isCurly {
		name, isIdent := firstToken.(Ident)
		custom := isIdent && strings.HasPrefix(name.Value, "--")
		maybeDeclaration := isIdent && startsWithColon(tokens)
		otherValue := false // a significant token other than the first block
		colon := false      // the ":" after the name, skipped
		for tokens.HasNext() {
			token := tokens.Next()
			if IsLiteral(token, ";") {
				semicolonToken = append(semicolonToken, token)
				break
			}
			declarationTokens = append(declarationTokens, token)
			_, isCurly := token.(CurlyBracketsBlock)
			if isCurly && firstBlock < 0 {
				firstBlock = len(declarationTokens)
			} else if k := token.Kind(); k != KWhitespace && k != KComment {
				switch {
				case !colon && maybeDeclaration:
					colon = true
				case IsLiteral(token, "!") && importantFollows(tokens):
					// !important is not a value
				default:
					otherValue = true
				}
			}
			if firstBlock >= 0 && (!maybeDeclaration || !custom && otherValue) {
				break
			}
		}
	}
	declaration := parseDeclaration(firstToken, NewIter(declarationTokens), true)
	if _, isDecl := declaration.(Declaration); isDecl {
		return declaration
	}
	if firstBlock >= 0 {
		// the rule ends with its block: what follows is read again
		tokens.index = start + firstBlock
		return consumeQualifiedRule(firstToken, NewIter(declarationTokens[:firstBlock]), true, true)
	}
	// declarationTokens stop before the first ";", so the qualified rule
	// never reads past semicolonToken: there is no need to copy the rest of
	// the input (quadratic otherwise).
	return consumeQualifiedRule(firstToken, NewIter(append(declarationTokens, semicolonToken...)), true, true)
}

// startsLikeCustomProperty reports whether the first two significant tokens
// are an identifier starting with "--" and a colon.
func startsLikeCustomProperty(tokens []Token) bool {
	i := firstSignificant(tokens, 0)
	if i < 0 {
		return false
	}
	ident, ok := tokens[i].(Ident)
	if !ok || !strings.HasPrefix(ident.Value, "--") {
		return false
	}
	j := firstSignificant(tokens, i+1)
	return j >= 0 && IsLiteral(tokens[j], ":")
}

// firstSignificant returns the index of the first token of tokens[from:]
// which is neither whitespace nor a comment, or -1.
func firstSignificant(tokens []Token, from int) int {
	for i := from; i < len(tokens); i++ {
		if k := tokens[i].Kind(); k != KWhitespace && k != KComment {
			return i
		}
	}
	return -1
}

// importantFollows reports whether the next significant tokens are
// "important" then ";" or the end, without consuming anything: the "!"
// before them is the important flag.
func importantFollows(tokens *TokensIter) bool {
	i := tokens.index
	next := func() Token {
		for ; i < len(tokens.tokens); i++ {
			if k := tokens.tokens[i].Kind(); k != KWhitespace && k != KComment {
				i++
				return tokens.tokens[i-1]
			}
		}
		return nil
	}
	ident, ok := next().(Ident)
	if !ok || utils.AsciiLower(ident.Value) != "important" {
		return false
	}
	end := next()
	return end == nil || IsLiteral(end, ";")
}

// startsWithColon reports whether the next significant token is ":", without
// consuming anything.
func startsWithColon(tokens *TokensIter) bool {
	for i := tokens.index; i < len(tokens.tokens); i++ {
		switch t := tokens.tokens[i]; t.Kind() {
		case KWhitespace, KComment:
		default:
			return IsLiteral(t, ":")
		}
	}
	return false
}

// ParseRuleList parses a non-top-level `rule list`.
//
// This is used for parsing the `AtRule.content` of nested rules like “@media“.
// This differs from `ParseStylesheet` in that top-level “<!--“ and “-->“ tokens are not ignored.
//
// If [skipComments] is true, ignores CSS comments at the top-level of the list.
//
// If [skipWhitespace] is true, ignores whitespace at the top-level of the list.
// Whitespace are still preserved in the `QualifiedRule.Prelude` and the `QualifiedRule.Content` of rules.
func ParseRuleList(input []Token, skipComments, skipWhitespace bool) []Compound {
	tokens := NewIter(input)
	var result []Compound
	for tokens.HasNext() {
		token := tokens.Next()
		switch token := token.(type) {
		case Whitespace:
			if !skipWhitespace {
				result = append(result, token)
			}
		case Comment:
			if !skipComments {
				result = append(result, token)
			}
		default:
			val := consumeRule(token, tokens)
			result = append(result, val)
		}
	}
	return result
}

// ParseStylesheet parses a stylesheet from tokens.
//
// This is used e.g. for a “<style>“ HTML element.
// This differs from `ParseRuleList` in that top-level “<!--“ && “-->“ tokens are ignored.
// This is a legacy quirk for the “<style>“ HTML element.
func ParseStylesheet(input []Token, skipComments, skipWhitespace bool) []Compound {
	iter := NewIter(input)
	var result []Compound
	for iter.HasNext() {
		token := iter.Next()
		switch token := token.(type) {
		case Whitespace:
			if !skipWhitespace {
				result = append(result, token)
			}
		case Comment:
			if !skipComments {
				result = append(result, token)
			}
		case Literal:
			if token.Value != "<!--" && token.Value != "-->" {
				result = append(result, consumeRule(token, iter))
			}
		default:
			result = append(result, consumeRule(token, iter))
		}
	}
	return result
}

// ParseStylesheetBytes tokenizes `input`, UTF-8 encoded, and calls
// `ParseStylesheet`. A leading byte order mark is removed, as the decoding
// of CSS Syntax 3 §3.2 does (decoding other encodings is the caller's: the
// protocol charset, @charset).
func ParseStylesheetBytes(input []byte, skipComments, skipWhitespace bool) []Compound {
	input = bytes.TrimPrefix(input, []byte("\xEF\xBB\xBF"))
	l := Tokenize(input, skipComments)
	return ParseStylesheet(l, skipComments, skipWhitespace)
}
