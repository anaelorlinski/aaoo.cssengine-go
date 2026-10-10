package stylesheet

import (
	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/logger"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
	"github.com/anaelorlinski/aaoo.cssengine-go/selector"
	"github.com/anaelorlinski/aaoo.cssengine-go/validation"
)

// @supports conditions (CSS Conditional Rules Level 3):
//
//	<supports-condition> = not <in-parens>
//	                     | <in-parens> [ and <in-parens> ]*
//	                     | <in-parens> [ or <in-parens> ]*
//	<in-parens> = ( <supports-condition> ) | ( <declaration> ) | <general-enclosed>
//
// A declaration test passes when the validator accepts it
// (validation.Supports); anything unrecognized — including functions
// such as selector(...) — evaluates to false, per <general-enclosed>.

// EvaluateSupports evaluates an @supports prelude. Malformed conditions
// evaluate to false and warn.
func EvaluateSupports(prelude []Token) bool {
	ok, valid := supportsCondition(parser.RemoveWhitespace(prelude))
	if !valid {
		logger.L().Warn("Invalid @supports condition", "condition", parser.Serialize(prelude))
		return false
	}
	return ok
}

func supportsCondition(tokens []Token) (result, valid bool) {
	if len(tokens) == 0 {
		return false, false
	}

	if name, ok := asIdent(tokens, 0); ok && name == "not" {
		inner, valid := supportsInParens(tokens, 1)
		if !valid || len(tokens) != 2 {
			return false, false
		}
		return !inner, true
	}

	first, valid := supportsInParens(tokens, 0)
	if !valid {
		return false, false
	}
	result = first

	connective := ""
	for i := 1; i < len(tokens); i += 2 {
		ident, ok := asIdent(tokens, i)
		if !ok || (ident != "and" && ident != "or") || (connective != "" && ident != connective) {
			return false, false
		}
		connective = ident
		next, valid := supportsInParens(tokens, i+1)
		if !valid {
			return false, false
		}
		if connective == "and" {
			result = result && next
		} else {
			result = result || next
		}
	}
	return result, true
}

// supportsInParens evaluates the <in-parens> production at index i.
func supportsInParens(tokens []Token, i int) (result, valid bool) {
	if i >= len(tokens) {
		return false, false
	}
	switch block := tokens[i].(type) {
	case parser.ParenthesesBlock:
		args := parser.RemoveWhitespace(block.Arguments)
		// nested condition: ( not ... ), ( (...) and (...) ), ( (...) )
		// (a function starts one too: (selector(p)), (selector(p) and (…)))
		if name, ok := asIdent(args, 0); (ok && name == "not") || startsWithParens(args) || startsWithFunction(args) {
			if inner, vld := supportsCondition(args); vld {
				return inner, true
			}
		}
		// declaration test: ( property: value )
		if decl, ok := parser.ParseOneDeclaration(block.Arguments).(parser.Declaration); ok {
			return validation.Supports(decl.Name, decl.Value), true
		}
		// unparseable content: <general-enclosed>, false but valid
		return false, true
	case parser.FunctionBlock:
		if utils.AsciiLower(block.Name) == "selector" {
			return supportsSelector(block.Arguments), true
		}
		// font-tech(...), font-format(...), ... : <general-enclosed>, false
		return false, true
	}
	return false, false
}

func startsWithFunction(tokens []Token) bool {
	if len(tokens) == 0 {
		return false
	}
	_, ok := tokens[0].(parser.FunctionBlock)
	return ok
}

func startsWithParens(tokens []Token) bool {
	if len(tokens) == 0 {
		return false
	}
	_, ok := tokens[0].(parser.ParenthesesBlock)
	return ok
}

// supportsSelector evaluates selector(<complex-selector>) (CSS Conditional
// 4 §4.1): whether the selector parses, as a single complex selector.
func supportsSelector(args []Token) bool {
	args = parser.TrimWhitespace(args)
	if len(args) == 0 {
		return false
	}
	for _, t := range args {
		if lit, ok := t.(parser.Literal); ok && lit.Value == "," {
			return false // a list, not one selector
		}
	}
	// the parts dropped by forgiving lists are unsupported too
	group, err := selector.ParseGroupStrict(parser.Serialize(args))
	return err == nil && len(group) == 1
}
