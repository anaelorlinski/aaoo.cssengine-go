package parser

import (
	"strings"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
)

var fuzzSeeds = []string{
	"", "-", "1-", "#-", "@-", "a: -", "f(a /* x", `.\31 a`, "1e39", "16777217n+1",
	"a { color: red; b { width: calc(1px + 2%) } }",
	"@media print { a { color: rgb(1 2 3 / 50%) } }",
	"@import url(foo.css); @font-face { unicode-range: U+0-7F, u+4??; }",
	`url( "a" ) url(a\)b) url(a"b) "unterminated` + "\n" + `'x\` + "\n" + "y'",
	"<!-- a{} --> b{c:d!important}",
	"color: oklch(0.5 1e39 90); color: hwb(0 20 30); color: #ff000080",
	strings.Repeat("(", 600) + strings.Repeat(")", 600),
	strings.Repeat("f([{", 200),
}

// FuzzTokenize checks that tokenizing, parsing and serializing arbitrary
// input never panics, and, for input without parse errors (whose
// serialization is lossy by design), that serialization is stable: the
// serialization of the tokens of a serialization is itself.
func FuzzTokenize(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, css string) {
		for _, skipComments := range []bool{false, true} {
			tokens := Tokenize([]byte(css), skipComments)
			out := Serialize(tokens)
			if again := Serialize(Tokenize([]byte(out), skipComments)); !hasParseError(tokens) && again != out {
				t.Fatalf("unstable serialization of %q:\n%q\n%q", css, out, again)
			}
			if d := nestingDepth(tokens); d > limits.NestingDepth {
				t.Fatalf("depth %d exceeds %d", d, limits.NestingDepth)
			}
			for _, tok := range tokens {
				TransformTokens(tok, func(tok Token) (Token, bool, bool) {
					ParseColor(tok)
					ParseFunction(tok)
					return tok, false, false
				})
			}
			ParseNth(tokens)
			ParseOneComponentValue(tokens)
			ParseOneDeclaration(tokens)
			ParseStylesheet(tokens, skipComments, false)
			ParseRuleList(tokens, skipComments, true)
			for _, c := range ParseBlocksContents(tokens, false) {
				if d, ok := c.(Declaration); ok {
					Serialize(d.Value)
				}
			}
			ParseDeclarationList(tokens, skipComments, false)
		}
	})
}

func hasParseError(tokens []Token) bool {
	found := false
	for _, tok := range tokens {
		TransformTokens(tok, func(tok Token) (Token, bool, bool) {
			_, isErr := tok.(ParseError)
			found = found || isErr
			return tok, false, found
		})
	}
	return found
}
