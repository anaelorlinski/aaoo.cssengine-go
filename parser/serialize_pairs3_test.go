package parser

import (
	"reflect"
	"testing"
)

// Serializing then tokenizing again gives the same tokens (third review,
// PA3-4).
func TestSerializeBadPairs3(t *testing.T) {
	for _, input := range []string{"#/**/-->", "u+1/**/url(x)", "u+1/**/u+1", "U/**/+a", "u/**/+1"} {
		tokens := Tokenize([]byte(input), true)
		out := Serialize(tokens)
		again := Tokenize([]byte(out), true)
		if !reflect.DeepEqual(kinds(tokens), kinds(again)) {
			t.Errorf("%q → %q: %v, then %v", input, out, kinds(tokens), kinds(again))
		}
	}
}

func kinds(tokens []Token) []string {
	var out []string
	for _, t := range tokens {
		out = append(out, t.Kind().String())
	}
	return out
}
