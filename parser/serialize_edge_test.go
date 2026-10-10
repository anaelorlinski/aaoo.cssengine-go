package parser

import (
	"fmt"
	"testing"
)

// P5: a hex escape was serialised without its terminating space.
func TestSerializeIdentifierLeadingDigit(t *testing.T) {
	for _, input := range []string{`.\31 a`, `#\31 23`, `\31 a`} {
		tokens := Tokenize([]byte(input), false)
		out := Serialize(tokens)
		if again := Serialize(Tokenize([]byte(out), false)); again != out {
			t.Fatalf("%q: serialisation is not stable: %q then %q", input, out, again)
		}
		if got, want := tokens[len(tokens)-1].(interface{ dump() interface{} }).dump(),
			Tokenize([]byte(out), false)[len(tokens)-1].(interface{ dump() interface{} }).dump(); !equalDump(got, want) {
			t.Fatalf("%q -> %q reparses as %v, want %v", input, out, want, got)
		}
	}
	if got := serializeIdentifier("1a"); got != `\31 a` {
		t.Fatalf("got %q", got)
	}
}

func equalDump(a, b interface{}) bool {
	sa, _ := marshalJSON([]TC{dumped{a}})
	sb, _ := marshalJSON([]TC{dumped{b}})
	return sa == sb
}

type dumped struct{ v interface{} }

func (d dumped) dump() interface{} { return d.v }

// P9: NewPercentage stored "50%" in Value and the serializer added another "%".
func TestSerializeNewPercentage(t *testing.T) {
	if got := Serialize([]Token{NewPercentage(50, Pos{})}); got != "50%" {
		t.Fatalf("got %q", got)
	}
	if got := Serialize([]Token{NewPercentage(12.5, Pos{})}); got != "12.5%" {
		t.Fatalf("got %q", got)
	}
}

// P12: serialising some parse errors or an empty identifier panicked.
func TestSerializeDoesNotPanic(t *testing.T) {
	tokens := []Token{
		ParseOneComponentValue(nil).(ParseError),
		ParseOneComponentValue(Tokenize([]byte("a b"), false)).(ParseError),
		ParseOneDeclaration(Tokenize([]byte("1: a"), false)).(ParseError),
		ParseError{kind: errInvalidNumber},
		ParseError{kind: errTooDeep},
		NewIdent("", Pos{}),
		AtKeyword{},
		Dimension{Unit: ""},
	}
	for _, tok := range tokens {
		Serialize([]Token{tok})
	}
	if got := ParseNth([]Token{NewIdent("", Pos{})}); got != nil {
		t.Fatalf("got %v", got)
	}
	if got := ParseNth([]Token{NewIdent("n", Pos{}), Number{}}); got != nil {
		t.Fatalf("got %v", got)
	}
}

// PC8–PC10: serialized tokens tokenize back to the same tokens.
func TestSerializeRoundTripEdges(t *testing.T) {
	for _, input := range []string{
		`1\65 5`, `1\65 -x`, `1\65 `, // dimensions with an e unit
		"1/**/%", "#/**/-", "-/**/-", "-/**/-->", // missing bad pairs
		`url(\1 )`, `url(\7f )`, `url(a\b b)`, // control characters in url()
	} {
		tokens := Tokenize([]byte(input), true)
		out := Serialize(tokens)
		again := Tokenize([]byte(out), true)
		if fmt.Sprint(again) != fmt.Sprint(tokens) {
			t.Errorf("%q serialized as %q: %v, then %v", input, out, tokens, again)
		}
	}
}
