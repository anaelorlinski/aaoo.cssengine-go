package parser

import (
	"math"
	"strings"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
)

// P1: a trailing "-" made isIdentStart read past the end of the input.
func TestTokenizeTrailingHyphen(t *testing.T) {
	for _, input := range []string{"-", "1-", "#-", "@-", "a: -", "x:-", "align-items: -", "width: #-", "(-", "f(1-"} {
		Tokenize([]byte(input), false)
		ParseDeclarationListString(input, false, false)
	}
	tokens := Tokenize([]byte("1-"), false)
	if len(tokens) != 2 || tokens[0].Kind() != KNumber || !IsLiteral(tokens[1], "-") {
		t.Fatalf("unexpected tokens for %q: %v", "1-", tokens)
	}
}

// P2: one recursion level per bracket overflowed the stack. Blocks nested
// deeper than limits.NestingDepth become a single parse error token.
func TestTokenizeDeepNesting(t *testing.T) {
	const n = 3_000_000
	for _, open := range []string{"(", "[", "{", "f("} {
		input := strings.Repeat(open, n)
		tokens := Tokenize([]byte(input), false)
		_ = Serialize(tokens)
		if d := nestingDepth(tokens); d > limits.NestingDepth {
			t.Fatalf("%q: depth %d exceeds %d", open, d, limits.NestingDepth)
		}
	}

	// Content after an over-deep block is still tokenized normally.
	deep := strings.Repeat("(", limits.NestingDepth+10) + `")" /* ) */ \) ` + strings.Repeat(")", limits.NestingDepth+10)
	tokens := Tokenize([]byte("a "+deep+" b"), true)
	if len(tokens) != 5 {
		t.Fatalf("expected 5 top-level tokens, got %d", len(tokens))
	}
	if last, ok := tokens[4].(Ident); !ok || last.Value != "b" {
		t.Fatalf("expected trailing ident b, got %v", tokens[4])
	}
	foundErr := false
	TransformTokens(tokens[2], func(tok Token) (Token, bool, bool) {
		if pe, ok := tok.(ParseError); ok && pe.kind == errTooDeep {
			foundErr = true
		}
		return tok, false, false
	})
	if !foundErr {
		t.Fatal("expected a too-deep parse error inside the nested block")
	}
	if d := nestingDepth(tokens); d != limits.NestingDepth {
		t.Fatalf("expected depth %d, got %d", limits.NestingDepth, d)
	}

	// Nesting up to the limit is kept intact.
	ok := strings.Repeat("(", limits.NestingDepth) + strings.Repeat(")", limits.NestingDepth)
	if got := Serialize(Tokenize([]byte(ok), false)); got != ok {
		t.Fatal("nesting at the limit was not preserved")
	}
}

func nestingDepth(tokens []Token) int {
	maxDepth := 0
	type frame struct {
		tokens []Token
		depth  int
	}
	stack := []frame{{tokens, 0}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		maxDepth = max(maxDepth, f.depth)
		for _, tok := range f.tokens {
			if kids, ok := Children(tok); ok {
				stack = append(stack, frame{kids, f.depth + 1})
			}
		}
	}
	return maxDepth
}

// P4: an unterminated comment inside a block left the position at the
// comment start, so its content was tokenized again in the parent.
func TestTokenizeUnterminatedCommentInBlock(t *testing.T) {
	tokens := Tokenize([]byte("f(a /* x"), false)
	if len(tokens) != 1 {
		t.Fatalf("expected a single function token, got %d tokens: %v", len(tokens), tokens)
	}
	fn := tokens[0].(FunctionBlock)
	if len(fn.Arguments) != 3 || fn.Arguments[2].Kind() != KComment {
		t.Fatalf("unexpected arguments %v", fn.Arguments)
	}
}

// P8: numbers out of the float32 range became ±Inf.
func TestTokenizeHugeNumbers(t *testing.T) {
	for _, input := range []string{"1e39", "-1e39", "1e999999", "1e39%", "1e39px", "999999999999999999999999999999999999999999999"} {
		tokens := Tokenize([]byte(input), false)
		var v float64
		switch tok := tokens[0].(type) {
		case Number:
			v = float64(tok.ValueF)
		case Percentage:
			v = float64(tok.ValueF)
		case Dimension:
			v = float64(tok.ValueF)
		default:
			t.Fatalf("%q: unexpected token %v", input, tok)
		}
		if math.IsInf(v, 0) || math.IsNaN(v) {
			t.Fatalf("%q: non-finite value %v", input, v)
		}
	}
}

// P11: Int went through float32 and lost precision.
func TestNumberIntPrecision(t *testing.T) {
	n := Tokenize([]byte("16777217"), false)[0].(Number)
	if !n.IsInt() || n.Int() != 16777217 {
		t.Fatalf("got %d (int: %v)", n.Int(), n.IsInt())
	}
	if nth := ParseNth(Tokenize([]byte("16777217n+16777217"), false)); nth == nil || *nth != [2]int{16777217, 16777217} {
		t.Fatalf("got %v", nth)
	}
	// Integers too large for int saturate instead of overflowing.
	big := Tokenize([]byte("99999999999999999999999"), false)[0].(Number)
	if !big.IsInt() || big.Int() != math.MaxInt64 {
		t.Fatalf("got %d (int: %v)", big.Int(), big.IsInt())
	}
}

// P13: flags were built with iota, so isErrorInString == isInteger|isIdentifier.
func TestFlagsAreDistinctBits(t *testing.T) {
	flags := []flag{isInteger, isIdentifier, isErrorInString, isErrorInURL}
	var seen flag
	for _, f := range flags {
		if f == 0 || f&(f-1) != 0 || seen&f != 0 {
			t.Fatalf("flag %d is not a distinct bit", f)
		}
		seen |= f
	}
	bad := Tokenize([]byte(`"abc`), false)[0].(String)
	if bad.isIdentifier() {
		t.Fatal("a bad string must not be flagged as an identifier")
	}
}

// P14: columns counted bytes instead of code points.
func TestTokenColumnsCountCodePoints(t *testing.T) {
	tokens := Tokenize([]byte("é é\nàà b"), false)
	want := []Pos{{1, 1}, {1, 2}, {1, 3}, {1, 4}, {2, 1}, {2, 3}, {2, 4}}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens", len(tokens))
	}
	for i, tok := range tokens {
		if tok.Pos() != want[i] {
			t.Errorf("token %d (%v): got %v, want %v", i, tok, tok.Pos(), want[i])
		}
	}
}

// PC1: "u+code" is the selector u + code, not the range U+C and "ode".
func TestUnicodeRangeTokenBoundary(t *testing.T) {
	for input, want := range map[string]string{
		"u+code": "u+code", "u+em": "u+em", "u+details": "u+details",
		"u+a": "U+A", "u+dd": "U+DD", "U+0-7F": "U+0-7F", "u+4??": "U+400-4FF",
	} {
		if got := Serialize(Tokenize([]byte(input), false)); got != want {
			t.Errorf("%q serialized as %q, want %q", input, got, want)
		}
	}
}
