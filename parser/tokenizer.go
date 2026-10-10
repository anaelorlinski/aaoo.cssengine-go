package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
)

// Pos is the position of a token in the input CSS file
type Pos struct {
	Line, Column int
}

// Token is a CSS component value, used to build declarations
type Token interface {
	Pos() Pos
	Kind() Kind
	serializeTo(writer io.StringWriter)

	isToken()
}

type stringVal struct {
	Value string
	pos   Pos
	flag  flag
}

type numberVal struct {
	stringVal
	ValueF float32
}

type listVal struct {
	Arguments []Token
	pos       Pos
}

type (
	Comment    struct{ stringVal }
	Whitespace struct{ stringVal }
	Ident      struct{ stringVal }
	AtKeyword  struct{ stringVal }
	Hash       struct{ stringVal }
	String     struct{ stringVal }
	URL        struct{ stringVal }
	// Literal is a delimiter or a raw value.
	Literal struct{ stringVal }

	UnicodeRange struct {
		Start, End uint32
		pos        Pos
	}

	Number     struct{ numberVal }
	Percentage struct{ numberVal }
	Dimension  struct {
		Unit string
		numberVal
	}

	// ParenthesesBlock and the following are the compound values.
	ParenthesesBlock    listVal
	SquareBracketsBlock listVal
	CurlyBracketsBlock  listVal
	FunctionBlock       struct {
		Name string
		listVal
	}

	// ParseError is the special token used for parse errors.
	ParseError struct {
		Message string
		kind    byte
		pos     Pos
	}
)

func (v Literal) Pos() Pos             { return v.pos }
func (v ParseError) Pos() Pos          { return v.pos }
func (v Comment) Pos() Pos             { return v.pos }
func (v Whitespace) Pos() Pos          { return v.pos }
func (v Ident) Pos() Pos               { return v.pos }
func (v AtKeyword) Pos() Pos           { return v.pos }
func (v Hash) Pos() Pos                { return v.pos }
func (v String) Pos() Pos              { return v.pos }
func (v URL) Pos() Pos                 { return v.pos }
func (v UnicodeRange) Pos() Pos        { return v.pos }
func (v Number) Pos() Pos              { return v.pos }
func (v Percentage) Pos() Pos          { return v.pos }
func (v Dimension) Pos() Pos           { return v.pos }
func (v ParenthesesBlock) Pos() Pos    { return v.pos }
func (v SquareBracketsBlock) Pos() Pos { return v.pos }
func (v CurlyBracketsBlock) Pos() Pos  { return v.pos }
func (v FunctionBlock) Pos() Pos       { return v.pos }

func (Literal) isToken()             {}
func (ParseError) isToken()          {}
func (Comment) isToken()             {}
func (Whitespace) isToken()          {}
func (Ident) isToken()               {}
func (AtKeyword) isToken()           {}
func (Hash) isToken()                {}
func (String) isToken()              {}
func (URL) isToken()                 {}
func (UnicodeRange) isToken()        {}
func (Number) isToken()              {}
func (Percentage) isToken()          {}
func (Dimension) isToken()           {}
func (ParenthesesBlock) isToken()    {}
func (SquareBracketsBlock) isToken() {}
func (CurlyBracketsBlock) isToken()  {}
func (FunctionBlock) isToken()       {}

func (Literal) Kind() Kind             { return KLitteral }
func (ParseError) Kind() Kind          { return KParseError }
func (Comment) Kind() Kind             { return KComment }
func (Whitespace) Kind() Kind          { return KWhitespace }
func (Ident) Kind() Kind               { return KIdent }
func (AtKeyword) Kind() Kind           { return KAtKeyword }
func (Hash) Kind() Kind                { return KHash }
func (String) Kind() Kind              { return KString }
func (URL) Kind() Kind                 { return KURL }
func (UnicodeRange) Kind() Kind        { return KUnicodeRange }
func (Number) Kind() Kind              { return KNumber }
func (Percentage) Kind() Kind          { return KPercentage }
func (Dimension) Kind() Kind           { return KDimension }
func (ParenthesesBlock) Kind() Kind    { return KParenthesesBlock }
func (SquareBracketsBlock) Kind() Kind { return KSquareBracketsBlock }
func (CurlyBracketsBlock) Kind() Kind  { return KCurlyBracketsBlock }
func (FunctionBlock) Kind() Kind       { return KFunctionBlock }

type Kind uint8

const (
	KLitteral Kind = iota
	KParseError
	KComment
	KWhitespace
	KIdent
	KAtKeyword
	KHash
	KString
	KURL
	KUnicodeRange
	KNumber
	KPercentage
	KDimension
	KParenthesesBlock
	KSquareBracketsBlock
	KCurlyBracketsBlock
	KFunctionBlock
)

func (k Kind) String() string {
	switch k {
	case KLitteral:
		return "litteral"
	case KParseError:
		return "parse-error"
	case KComment:
		return "comment"
	case KWhitespace:
		return "whitespace"
	case KIdent:
		return "ident"
	case KAtKeyword:
		return "at-keyword"
	case KHash:
		return "hash"
	case KString:
		return "string"
	case KURL:
		return "url"
	case KUnicodeRange:
		return "unicode-range"
	case KNumber:
		return "number"
	case KPercentage:
		return "percentage"
	case KDimension:
		return "dimension"
	case KParenthesesBlock:
		return "() block"
	case KSquareBracketsBlock:
		return "[] block"
	case KCurlyBracketsBlock:
		return "{} block"
	case KFunctionBlock:
		return "function"
	default:
		panic("exhaustive type switch")
	}
}

func NewIdent(v string, pos Pos) Ident { return Ident{stringVal{Value: v, pos: pos}} }

func NewLiteral(v string, pos Pos) Literal { return Literal{stringVal{Value: v, pos: pos}} }

func NewWhitespace(v string, pos Pos) Whitespace { return Whitespace{stringVal{Value: v, pos: pos}} }

func NewNumber(v float32, pos Pos) Number {
	isInt := v == float32(math.Trunc((float64(v))))
	repr := fmt.Sprintf("%v", v)
	return newNumber(repr, v, isInt, pos)
}

func NewPercentage(v float32, pos Pos) Percentage {
	isInt := v == float32(math.Trunc((float64(v))))
	repr := fmt.Sprintf("%v", v)
	return Percentage{numberVal{stringVal{Value: repr, pos: pos, flag: newFlag(isInteger, isInt)}, v}}
}

func newNumber(v string, vf float32, isInt bool, pos Pos) Number {
	return Number{numberVal{stringVal{Value: v, pos: pos, flag: newFlag(isInteger, isInt)}, vf}}
}

func NewDimension(nb Number, dim string) Dimension { return Dimension{dim, nb.numberVal} }

func NewFunctionBlock(pos Pos, name string, arguments []Token) FunctionBlock {
	return FunctionBlock{Name: name, listVal: listVal{arguments, pos}}
}

// bitmask for misc token properties
type flag uint8

const (
	isInteger flag = 1 << iota
	isIdentifier
	isErrorInString
	isErrorInURL
)

func newFlag(v flag, condition bool) flag {
	var f flag
	if condition {
		f |= v
	}
	return f
}

func (tk stringVal) isIdentifier() bool { return tk.flag&isIdentifier != 0 }
func (tk stringVal) isError() bool      { return tk.flag&isErrorInString != 0 }

// IsInt returns true for numerical token with integer value.
func (tk numberVal) IsInt() bool { return tk.flag&isInteger != 0 }

// Int assumes a numeric token. Integers are read from their representation,
// so that values above 2^24 keep their precision (ValueF is a float32);
// integers out of the int range saturate.
func (tk numberVal) Int() int {
	if tk.IsInt() {
		v, err := strconv.ParseInt(tk.Value, 10, 0)
		if err == nil || errors.Is(err, strconv.ErrRange) {
			return int(v)
		}
	}
	return int(tk.ValueF)
}

const (
	errBadString     byte = 'b'
	errBadURL        byte = 'u'
	errP             byte = ')'
	errB             byte = ']'
	errC             byte = '}'
	errEofInString   byte = 's'
	errEofInUrl      byte = 'e'
	errInvalidNumber byte = 'n'
	errEmpty         byte = 'E'
	errExtraInput    byte = 'x'
	errInvalid       byte = 'i'
	errTooDeep       byte = 'd'
)

// -------------------------- tokenizer --------------------------

const (
	charUnicodeRange = "0123456789abcdefABCDEF"
	nonPrintable     = "\"'(\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f"
)

var (
	numberRe    = regexp.MustCompile(`^[-+]?([0-9]*\.)?[0-9]+([eE][+-]?[0-9]+)?`)
	hexEscapeRe = regexp.MustCompile(`^([0-9A-Fa-f]{1,6})[ \n\t]?`)
)

type tokenizer struct {
	src         []byte
	pos         int
	previousPos int // start of the previous token
	line        int
	column      int // of previousPos, in code points, starting at 1
	depth       int // number of enclosing blocks

	skipComments bool
}

func (tk *tokenizer) consumeIdent() string {
	// http://drafts.csswg.org/csswg/css-syntax/#consume-a-name
	var chunks strings.Builder
	L := len(tk.src)
	startPos := tk.pos
	for tk.pos < L {
		c, w := utf8.DecodeRune(tk.src[tk.pos:])
		if strings.ContainsRune("abcdefghijklmnopqrstuvwxyz-_0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ", c) || c > 0x7F {
			tk.pos += w
		} else if c == '\\' && !bytes.HasPrefix(tk.src[tk.pos:], []byte("\\\n")) {
			// Valid escape
			chunks.Write(tk.src[startPos:tk.pos])
			tk.pos += w
			car := tk.consumeEscape()
			chunks.WriteRune(car)
			startPos = tk.pos
		} else {
			break
		}
	}
	chunks.Write(tk.src[startPos:tk.pos])
	return chunks.String()
}

// Return the range
// http://drafts.csswg.org/csswg/css-syntax/#consume-a-unicode-range-token
func (tk *tokenizer) consumeUnicodeRange() (start, end int64, err error) {
	length := len(tk.src)
	startPos := tk.pos
	maxPos := min(tk.pos+6, length)
	var startS, endS string
	for tk.pos < maxPos {
		r, w := utf8.DecodeRune(tk.src[tk.pos:])
		if !strings.ContainsRune(charUnicodeRange, r) {
			break
		}
		tk.pos += w
	}
	startS = string(tk.src[startPos:tk.pos])
	questionMarks := 0
	// Same maxPos as before: total of hex digits && question marks <= 6
	for tk.pos < maxPos {
		r, w := utf8.DecodeRune(tk.src[tk.pos:])
		if r != '?' {
			break
		}
		tk.pos += w
		questionMarks += 1
	}

	if questionMarks != 0 {
		endS = startS + strings.Repeat("F", questionMarks)
		startS = startS + strings.Repeat("0", questionMarks)
	} else if tk.pos+1 < length && tk.src[tk.pos] == '-' && strings.ContainsRune(charUnicodeRange, rune(tk.src[tk.pos+1])) {
		tk.pos += utf8.RuneLen(rune(tk.src[tk.pos+1]))
		startPos = tk.pos
		maxPos = min(tk.pos+6, length)
		for tk.pos < maxPos {
			r, w := utf8.DecodeRune(tk.src[tk.pos:])
			if !strings.ContainsRune(charUnicodeRange, r) {
				break
			}
			tk.pos += w
		}
		endS = string(tk.src[startPos:tk.pos])
	} else {
		endS = startS
	}
	start, err = strconv.ParseInt(startS, 16, 0)
	if err != nil {
		return 0, 0, err
	}
	end, err = strconv.ParseInt(endS, 16, 0)
	return start, end, err
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\n' || r == '\t'
}

func eofInURL(pos Pos) ParseError {
	return ParseError{pos: pos, kind: errEofInUrl, Message: "eof-in-url"}
}

// http://drafts.csswg.org/csswg/css-syntax/#consume-a-url-token
// returns an optionnal URL and an optionnal ParseError
func (tk *tokenizer) consumeUrl(pos Pos) (Token, Token) {
	L := len(tk.src)
	// Skip whitespace
	for tk.pos < L && isSpace(rune(tk.src[tk.pos])) {
		tk.pos += 1
	}
	if tk.pos >= L { // EOF
		return URL{stringVal{pos: pos, Value: "", flag: isErrorInURL}}, eofInURL(pos)
	}

	var value string
	c := rune(tk.src[tk.pos])
	if c == '"' || c == '\'' {
		var err byte
		value, _, err = tk.consumeQuotedString()
		if err != 0 {
			goto badURL
		}
	} else if c == ')' {
		tk.pos += 1
		return URL{stringVal{pos: pos, Value: ""}}, nil
	} else {
		var chunks strings.Builder
		startPos := tk.pos
	mainLoop:
		for {
			if tk.pos >= L { // EOF
				chunks.Write(tk.src[startPos:tk.pos])
				return URL{stringVal{pos: pos, Value: chunks.String(), flag: isErrorInURL}}, eofInURL(pos)
			}
			c, w := utf8.DecodeRune(tk.src[tk.pos:])
			switch {
			case c == ')':
				chunks.Write(tk.src[startPos:tk.pos])
				tk.pos += w
				return URL{stringVal{pos: pos, Value: chunks.String()}}, nil
			case c == ' ' || c == '\n' || c == '\t':
				chunks.Write(tk.src[startPos:tk.pos])
				value = chunks.String()
				tk.pos += w
				break mainLoop
			case c == '\\' && !bytes.HasPrefix(tk.src[tk.pos:], []byte("\\\n")):
				// Valid escape
				chunks.Write(tk.src[startPos:tk.pos])
				tk.pos += w
				cs := tk.consumeEscape()
				chunks.WriteRune(cs)
				startPos = tk.pos
			case c == '\\':
				// a "\" before a newline is not an escape: a bad url
				tk.pos += w
				goto badURL
			default:
				tk.pos += w
				// http://drafts.csswg.org/csswg/css-syntax/#non-printable-character
				if strings.ContainsRune(nonPrintable, c) {
					goto badURL
				}
			}
		}
	}

	for tk.pos < L {
		r, w := utf8.DecodeRune(tk.src[tk.pos:])
		if isSpace(r) {
			tk.pos += w
		} else {
			break
		}
	}
	if tk.pos < L {
		if tk.src[tk.pos] == ')' {
			tk.pos += 1
			return URL{stringVal{pos: pos, Value: value}}, nil
		}
	} else { // EOF
		return URL{stringVal{pos: pos, Value: value, flag: isErrorInURL}}, eofInURL(pos)
	}

badURL:
	// http://drafts.csswg.org/csswg/css-syntax/#consume-the-remnants-of-a-bad-url0
	for tk.pos < L {
		if bytes.HasPrefix(tk.src[tk.pos:], []byte("\\)")) {
			tk.pos += 2
		} else if tk.src[tk.pos] == ')' {
			tk.pos += 1
			break
		} else {
			_, w := utf8.DecodeRune(tk.src[tk.pos:])
			tk.pos += w
		}
	}
	return nil, ParseError{pos: pos, kind: errBadURL, Message: "bad-url"}
}

// assumes we are at a quote and returns the unescaped value
// also returns wheter or not the token is valid
// http://drafts.csswg.org/csswg/css-syntax/#consume-a-string-token
func (tk *tokenizer) consumeQuotedString() (string, bool, byte) {
	quote := rune(tk.src[tk.pos])
	tk.pos += 1

	var (
		L         = len(tk.src)
		startPos  = tk.pos
		hasBroken = false
		chunks    strings.Builder
	)
mainLoop:
	for tk.pos < L {
		c, w := utf8.DecodeRune(tk.src[tk.pos:])
		switch c {
		case quote:
			chunks.Write(tk.src[startPos:tk.pos])
			tk.pos += w
			hasBroken = true
			break mainLoop
		case '\\':
			chunks.Write(tk.src[startPos:tk.pos])
			tk.pos += w
			if tk.pos < L {
				if tk.src[tk.pos] == '\n' { // Ignore escaped newlines
					tk.pos += 1
				} else {
					cs := tk.consumeEscape()
					chunks.WriteRune(cs)
				}
			} // else: Escaped EOF, do nothing
			startPos = tk.pos
		case '\n': // Unescaped newline
			return "", false, errBadString // bad-string
		default:
			tk.pos += w
		}
	}

	err := byte(0)
	if !hasBroken {
		chunks.Write(tk.src[startPos:tk.pos])
		err = errEofInString
	}
	return chunks.String(), true, err
}

// Return the unescaped char
// Assumes a valid escape: pos is just after '\' and not followed by '\n'.
func (tk *tokenizer) consumeEscape() rune {
	// http://drafts.csswg.org/csswg/css-syntax/#consume-an-escaped-character
	hexMatch := hexEscapeRe.FindSubmatch(tk.src[tk.pos:])
	if len(hexMatch) >= 2 {
		codepoint, err := strconv.ParseInt(string(hexMatch[1]), 16, 0)
		if err != nil {
			// the regexp ensure its a valid hex number
			panic(fmt.Sprintf("codepoint should be valid hexadecimal, got %s", hexMatch[0]))
		}
		char := '\uFFFD'
		if 0 < codepoint && codepoint <= unicode.MaxRune {
			char = rune(codepoint)
		}
		tk.pos += len(hexMatch[0])
		return char
	} else if tk.pos < len(tk.src) {
		r, w := utf8.DecodeRune(tk.src[tk.pos:])
		tk.pos += w
		return r
	} else {
		return '\uFFFD'
	}
}

// assume we are at a space
func (tk *tokenizer) consumeWhitespace() string {
	tokenStartPos := tk.pos
	tk.pos += 1 // skip the first space
	for ; tk.pos < len(tk.src); tk.pos += 1 {
		if u := tk.src[tk.pos]; !isSpace(rune(u)) {
			break
		}
	}
	return string(tk.src[tokenStartPos:tk.pos])
}

// assume we are at 'U' or 'u'
func (tk *tokenizer) tryConsumeUnicodeRune(pos Pos) (Token, bool) {
	if tk.pos+2 < len(tk.src) && tk.src[tk.pos+1] == '+' && strings.ContainsRune("0123456789abcdefABCDEF?", rune(tk.src[tk.pos+2])) {
		begin := tk.pos
		tk.pos += 2
		start, end, err := tk.consumeUnicodeRange()
		// A range must end at a token boundary: "u+code" (a selector, u
		// followed by code) is not the range U+C followed by "ode".
		// (CSS Syntax 3 now tokenizes unicode ranges as ordinary tokens,
		// re-parsed by the unicode-range descriptor; tinycss2 and its test
		// corpus keep the token.)
		// A digit may follow: a range has at most 6 of them.
		if tk.pos < len(tk.src) && (isNameStart(tk.src, tk.pos) || tk.src[tk.pos] == '\\') {
			tk.pos = begin
			return nil, false
		}
		if err != nil {
			return ParseError{pos: pos, kind: errInvalidNumber, Message: err.Error()}, true
		}

		return UnicodeRange{pos: pos, Start: uint32(start), End: uint32(end)}, true
	}
	return nil, false
}

// Return true if the given character is a name-start code point.
func isNameStart(css []byte, pos int) bool {
	// https://www.w3.org/TR/css-syntax-3/#name-start-code-point
	c, _ := utf8.DecodeRune(css[pos:])
	return c > 0x7F || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_'
}

// assumes we are in range and
// returns true if the current position is the start of a CSS identifier.
func (tk *tokenizer) isIdentStart() bool {
	// https://www.w3.org/TR/css-syntax-3/#would-start-an-identifier
	if isNameStart(tk.src, tk.pos) {
		return true
	} else if tk.src[tk.pos] == '-' {
		pos := tk.pos + 1
		// Name-start code point
		nameStart := pos < len(tk.src) && (isNameStart(tk.src, pos) || tk.src[pos] == '-')
		// Valid escape
		validEscape := pos < len(tk.src) && tk.src[pos] == '\\' && !bytes.HasPrefix(tk.src[pos:], []byte("\\\n"))
		return nameStart || validEscape
	} else if tk.src[tk.pos] == '\\' {
		return !bytes.HasPrefix(tk.src[tk.pos:], []byte("\\\n"))
	}
	return false
}

// may return nil if not a number
func (tk *tokenizer) tryConsumeNumber(pos Pos) Token {
	match := numberRe.FindIndex(tk.src[tk.pos:])
	if match == nil {
		return nil
	}

	value := string(tk.src[tk.pos+match[0] : tk.pos+match[1]])
	tk.pos += match[1]
	valueF, _ := strconv.ParseFloat(value, 32)
	if valueF == 0 {
		valueF = 0. // workaround -0
	} else if math.IsInf(valueF, 0) {
		// Out of the float32 range: clamp to the closest supported value
		// (https://drafts.csswg.org/css-values-4/#numeric-ranges).
		valueF = math.Copysign(math.MaxFloat32, valueF)
	}
	n := numberVal{
		stringVal{
			Value: value,
			pos:   pos,
			flag:  newFlag(isInteger, !strings.ContainsAny(value, ".eE")),
		},
		float32(valueF),
	}
	L := len(tk.src)
	if tk.pos < L && tk.isIdentStart() {
		// Per WP commit dbde3d98: CSS dimension units are
		// case-insensitive ("PX" == "px"). Lower-case at parse time
		// so downstream comparisons in unit tables stay simple.
		unit := utils.AsciiLower(tk.consumeIdent())
		return Dimension{unit, n}
	} else if tk.pos < L && tk.src[tk.pos] == '%' {
		tk.pos += 1
		return Percentage{n}
	} else {
		return Number{n}
	}
}

func (tk *tokenizer) tryConsumeHash(pos Pos) (Hash, bool) {
	if tk.pos >= len(tk.src) {
		return Hash{}, false
	}

	r, _ := utf8.DecodeRune(tk.src[tk.pos:])
	if ('0' <= r && r <= '9' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '-' || r == '_') ||
		r > 0x7F || // Non-ASCII
		(r == '\\' && !bytes.HasPrefix(tk.src[tk.pos:], []byte("\\\n"))) { // Valid escape
		flag := newFlag(isIdentifier, tk.isIdentStart())
		ident := tk.consumeIdent()
		return Hash{stringVal{pos: pos, Value: ident, flag: flag}}, true
	}

	return Hash{}, false
}

func (tk *tokenizer) consumeDelimOrLitteral(pos Pos) Literal {
	c := tk.src[tk.pos]
	switch {
	case bytes.HasPrefix(tk.src[tk.pos:], []byte("<!--")):
		tk.pos += 4
		return Literal{stringVal{pos: pos, Value: "<!--"}}
	case bytes.HasPrefix(tk.src[tk.pos:], []byte("||")):
		tk.pos += 2
		return Literal{stringVal{pos: pos, Value: "||"}}
	case c == '~' || c == '|' || c == '^' || c == '$' || c == '*':
		tk.pos += 1
		if bytes.HasPrefix(tk.src[tk.pos:], []byte{'='}) {
			tk.pos += 1
			return Literal{stringVal{pos: pos, Value: string(rune(c)) + "="}}
		} else {
			return Literal{stringVal{pos: pos, Value: string(rune(c))}}
		}
	default:
		r, w := utf8.DecodeRune(tk.src[tk.pos:])
		tk.pos += w
		return Literal{stringVal{pos: pos, Value: string(r)}}
	}
}

// updateLine returns the position of tk.pos. Columns count code points, as
// tinycss2 does, and are updated incrementally from the previous token.
func (tk *tokenizer) updateLine() Pos {
	newline := bytes.LastIndexByte(tk.src[tk.previousPos:tk.pos], '\n')
	if newline != -1 {
		newline += tk.previousPos
		tk.line += 1 + bytes.Count(tk.src[tk.previousPos:newline], []byte{'\n'})
		// First character in a line is in column 1.
		tk.column = 1 + utf8.RuneCount(tk.src[newline+1:tk.pos])
	} else {
		tk.column += utf8.RuneCount(tk.src[tk.previousPos:tk.pos])
	}
	tk.previousPos = tk.pos

	return Pos{tk.line, tk.column}
}

// consumeNested consumes the content of a block whose opening delimiter has
// just been consumed, up to [endChar]. Beyond [limits.NestingDepth], the block
// is skipped and ok is false.
func (tk *tokenizer) consumeNested(endChar byte) (content []Token, ok bool) {
	if tk.depth >= limits.NestingDepth {
		tk.skipBlock(endChar)
		return nil, false
	}
	tk.depth++
	content = tk.consumeValueList(endChar)
	tk.depth--
	return content, true
}

// skipBlock advances past the end of a block whose opening delimiter has just
// been consumed, without building tokens (and without recursion). Strings,
// comments and escapes are skipped so that the delimiters they contain are
// ignored; like in consumeValueList, an unmatched closing delimiter does not
// close the block. The only approximation is for bad unquoted url()s, whose
// remnants are not special-cased.
func (tk *tokenizer) skipBlock(endChar byte) {
	closers := []byte{endChar}
	L := len(tk.src)
	for tk.pos < L && len(closers) > 0 {
		switch c := tk.src[tk.pos]; c {
		case '(':
			closers = append(closers, ')')
			tk.pos += 1
		case '[':
			closers = append(closers, ']')
			tk.pos += 1
		case '{':
			closers = append(closers, '}')
			tk.pos += 1
		case ')', ']', '}':
			if c == closers[len(closers)-1] {
				closers = closers[:len(closers)-1]
			}
			tk.pos += 1
		case '\'', '"':
			tk.consumeQuotedString() // stops before an unescaped newline
		case '\\':
			tk.pos += 1
			if tk.pos < L {
				_, w := utf8.DecodeRune(tk.src[tk.pos:])
				tk.pos += w
			}
		case '/':
			if bytes.HasPrefix(tk.src[tk.pos:], []byte("/*")) {
				index := bytes.Index(tk.src[tk.pos+2:], []byte("*/"))
				if index == -1 {
					tk.pos = L
				} else {
					tk.pos += 2 + index + 2
				}
			} else {
				tk.pos += 1
			}
		default:
			// Non-ASCII bytes never match the ASCII cases above.
			tk.pos += 1
		}
	}
}

func tooDeep(pos Pos) ParseError {
	return ParseError{pos: pos, kind: errTooDeep, Message: "Blocks nested too deeply"}
}

// stops and returns when encountering [endChar]
func (tk *tokenizer) consumeValueList(endChar byte) []Token {
	var (
		L   = len(tk.src)
		out []Token // possibly nested tokens
	)

	for tk.pos < L {
		tokenPos := tk.updateLine()

		c := tk.src[tk.pos]
		switch c {
		case ' ', '\n', '\t':
			ws := tk.consumeWhitespace()
			out = append(out, Whitespace{stringVal{pos: tokenPos, Value: ws}})
			continue
		case 'U', 'u':
			if val, ok := tk.tryConsumeUnicodeRune(tokenPos); ok {
				out = append(out, val)
				continue
			}
		}

		if bytes.HasPrefix(tk.src[tk.pos:], []byte("-->")) { // Check before identifiers
			out = append(out, Literal{stringVal{pos: tokenPos, Value: "-->"}})
			tk.pos += 3
			continue
		} else if tk.isIdentStart() {
			value := tk.consumeIdent()
			if !(tk.pos < L && tk.src[tk.pos] == '(') { // Not a function
				out = append(out, Ident{stringVal{pos: tokenPos, Value: value}})
				continue
			}
			tk.pos += 1 // Skip the "("
			if utils.AsciiLower(value) == "url" {
				urlPos := tk.pos
				for urlPos < L && isSpace(rune(tk.src[urlPos])) {
					urlPos += 1
				}
				if urlPos >= L || (tk.src[urlPos] != '"' && tk.src[urlPos] != '\'') {
					val, err := tk.consumeUrl(tokenPos)
					if val != nil {
						out = append(out, val)
					}
					if err != nil {
						out = append(out, err)
					}
					continue
				}
			}

			// recurse
			arguments, ok := tk.consumeNested(')')
			if !ok {
				out = append(out, tooDeep(tokenPos))
				continue
			}
			out = append(out, FunctionBlock{value, listVal{arguments, tokenPos}})
			continue
		}

		if val := tk.tryConsumeNumber(tokenPos); val != nil {
			out = append(out, val)
			continue
		}

		switch c {
		case '@':
			tk.pos += 1
			if tk.pos < L && tk.isIdentStart() {
				ident := tk.consumeIdent()
				out = append(out, AtKeyword{stringVal{pos: tokenPos, Value: ident}})
			} else {
				out = append(out, Literal{stringVal{pos: tokenPos, Value: "@"}})
			}
		case '#':
			tk.pos += 1
			if hash, ok := tk.tryConsumeHash(tokenPos); ok {
				out = append(out, hash)
				continue
			}
			out = append(out, Literal{stringVal{pos: tokenPos, Value: "#"}})
		case '{':
			tk.pos += 1
			if arguments, ok := tk.consumeNested('}'); ok {
				out = append(out, CurlyBracketsBlock{arguments, tokenPos})
			} else {
				out = append(out, tooDeep(tokenPos))
			}
		case '[':
			tk.pos += 1
			if arguments, ok := tk.consumeNested(']'); ok {
				out = append(out, SquareBracketsBlock{arguments, tokenPos})
			} else {
				out = append(out, tooDeep(tokenPos))
			}
		case '(':
			tk.pos += 1
			if arguments, ok := tk.consumeNested(')'); ok {
				out = append(out, ParenthesesBlock{arguments, tokenPos})
			} else {
				out = append(out, tooDeep(tokenPos))
			}
		case 0: // remove this case to avoid false comparaison with endChar
		case endChar: // Matching }, ] or ), or 0
			// consume the delimiter and return
			tk.pos += 1
			return out
		case '}', ']', ')':
			// unexpected delimiter
			out = append(out, ParseError{pos: tokenPos, kind: c, Message: "Unmatched " + string(rune(c))})
			tk.pos += 1
		case '\'', '"':
			quotedString, addValue, err := tk.consumeQuotedString()
			if addValue {
				out = append(out, String{stringVal{pos: tokenPos, Value: quotedString, flag: newFlag(isErrorInString, err != 0)}})
			}
			if err != 0 {
				out = append(out, ParseError{pos: tokenPos, kind: err, Message: "bad string token"})
			}
		default:
			if bytes.HasPrefix(tk.src[tk.pos:], []byte("/*")) { // Comment
				index := bytes.Index(tk.src[tk.pos+2:], []byte("*/"))
				if index == -1 {
					if !tk.skipComments {
						out = append(out, Comment{stringVal{pos: tokenPos, Value: string(tk.src[tk.pos+2:])}})
					}
					tk.pos = len(tk.src) // also ends the enclosing blocks
					return out
				}
				tk.pos += 2 + index
				if !tk.skipComments {
					out = append(out, Comment{stringVal{pos: tokenPos, Value: string(tk.src[tk.previousPos+2 : tk.pos])}})
				}
				tk.pos += 2
			} else {
				v := tk.consumeDelimOrLitteral(tokenPos)
				out = append(out, v)
			}
		}
	}

	return out
}

// Tokenize parses a list of component values.
//
// If `skipComments` is true, it ignores CSS comments:
// the return values (and recursively its blocks and functions)
// will not contain any `Comment` object.
func Tokenize(css []byte, skipComments bool) []Token {
	// This turns out to be faster than a regexp:
	css = bytes.ReplaceAll(css, []byte("\u0000"), []byte("\uFFFD"))
	css = bytes.ReplaceAll(css, []byte("\r\n"), []byte("\n"))
	css = bytes.ReplaceAll(css, []byte("\r"), []byte("\n"))
	css = bytes.ReplaceAll(css, []byte("\f"), []byte("\n"))

	tk := tokenizer{
		src: css, pos: 0,
		previousPos: 0,
		line:        1, column: 1,
		skipComments: skipComments,
	}

	return tk.consumeValueList(0)
}

type TokensIter struct {
	tokens []Token
	index  int
}

func NewIter(tokens []Token) *TokensIter {
	return &TokensIter{tokens, 0}
}

func (it TokensIter) HasNext() bool {
	return it.index < len(it.tokens)
}

// Next returns the Next token or nil at the end
func (it *TokensIter) Next() (t Token) {
	if it.HasNext() {
		t = it.tokens[it.index]
		it.index += 1
	}
	return t
}

// NextSignificant returns the next significant (neither whitespace or comment) token,
// or nil
func (it *TokensIter) NextSignificant() Token {
	for it.HasNext() {
		token := it.Next()
		switch token.(type) {
		case Whitespace, Comment:
			continue
		default:
			return token
		}
	}
	return nil
}

// ParseOneComponentValue parses a single `component value`.
// This is used e.g. for an attribute value referred to by “attr(foo length)“.
func ParseOneComponentValue(input []Token) Token {
	tokens := NewIter(input)
	first := tokens.NextSignificant()
	if first == nil {
		return ParseError{pos: Pos{1, 1}, kind: errEmpty, Message: "Input is empty"}
	}
	second := tokens.NextSignificant()
	if second != nil {
		return ParseError{pos: second.Pos(), kind: errExtraInput, Message: "Got more than one token"}
	}
	return first
}

func parseOneComponentValue(css []byte, skipComments bool) Token {
	return ParseOneComponentValue(Tokenize(css, skipComments))
}

// RemoveWhitespace removes any [Whitespace] and [Comment] tokens from the list.
func RemoveWhitespace(tokens []Token) []Token {
	var out []Token
	for _, token := range tokens {
		if token.Kind() != KWhitespace && token.Kind() != KComment {
			out = append(out, token)
		}
	}
	return out
}

// TrimWhitespace removes the leading and trailing [Whitespace] and
// [Comment] tokens of the list.
func TrimWhitespace(tokens []Token) []Token {
	isSpace := func(t Token) bool { return t.Kind() == KWhitespace || t.Kind() == KComment }
	for len(tokens) > 0 && isSpace(tokens[0]) {
		tokens = tokens[1:]
	}
	for len(tokens) > 0 && isSpace(tokens[len(tokens)-1]) {
		tokens = tokens[:len(tokens)-1]
	}
	return tokens
}

// SplitOnComma splits a list of tokens on commas, ie on [Literal] ",".
func SplitOnComma(tokens []Token) [][]Token {
	var parts [][]Token
	var thisPart []Token
	for _, token := range tokens {
		litteral, ok := token.(Literal)
		if ok && litteral.Value == "," {
			parts = append(parts, thisPart)
			thisPart = nil
		} else {
			thisPart = append(thisPart, token)
		}
	}
	parts = append(parts, thisPart)
	return parts
}

// ParseFunction parses functional notation.
//
// Return “(name, args)“ if the given token is a function with comma or
// space-separated arguments. Return zero values otherwise.
func ParseFunction(functionToken_ Token) (string, []Token) {
	functionToken, ok := functionToken_.(FunctionBlock)
	if !ok {
		return "", nil
	}
	content := RemoveWhitespace(functionToken.Arguments)
	var (
		arguments []Token
		token     Token
	)
	lastIsComma := false
	for len(content) > 0 {
		token, content = content[0], content[1:]
		isComma := IsLiteral(token, ",")
		if lastIsComma && isComma {
			return "", nil
		}
		if isComma {
			lastIsComma = true
		} else {
			lastIsComma = false
			if fn, isFunc := token.(FunctionBlock); isFunc {
				innerName, _ := ParseFunction(fn)
				if innerName == "" {
					return "", nil
				}
			}
			arguments = append(arguments, token)
		}
	}
	if lastIsComma {
		return "", nil
	}
	return utils.AsciiLower(functionToken.Name), arguments
}
