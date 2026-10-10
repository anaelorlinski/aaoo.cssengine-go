package selector

import (
	"reflect"
	"testing"
)

func TestSerialize(t *testing.T) {
	var testSer []string
	for _, test := range selectorTests {
		testSer = append(testSer, test.selector)
	}
	for _, test := range testsPseudo {
		testSer = append(testSer, test.selector)
	}
	for _, test := range loadValidSelectors(t) {
		if test.Xfail {
			continue
		}
		testSer = append(testSer, test.Selector)
	}

	for _, test := range testSer {
		s, err := ParseGroup(test)
		if err != nil {
			t.Fatalf("error compiling %q: %s", test, err)
		}

		serialized := s.String()
		s2, err := ParseGroup(serialized)
		if err != nil {
			t.Errorf("error compiling %q: %s %T (original : %s)", serialized, err, s, test)
		}
		if !reflect.DeepEqual(s, s2) {
			t.Errorf("can't retrieve selector from serialized : %s (original : %s, sel : %#v)", serialized, test, s)
		}
	}
}

// Serialized selectors parse back to the same selector (CSSOM escaping of
// identifiers and strings).
func TestSerializeEscapes(t *testing.T) {
	for _, input := range []string{
		`[a="x\"y"]`, `:contains("a\"b")`, `[data-x="a\\b"]`, `.\31 23`, `#\-`, `.-\31 a`,
		`.a\:b`, `[a="line\a break"]`, `:lang("de\"x")`, `.\7f x`,
	} {
		sel, err := ParseGroup(input)
		if err != nil {
			t.Fatalf("%s: %s", input, err)
		}
		out := sel.String()
		again, err := ParseGroup(out)
		if err != nil {
			t.Errorf("%s: serialized as %q, which does not parse: %s", input, out, err)
			continue
		}
		if again.String() != out {
			t.Errorf("%s: %q, then %q", input, out, again.String())
		}
	}
	// the values survive
	doc := MustParseHTML(`<p class="123" data-x="a\b" id="-">x</p>`)
	p := Query(doc, MustCompile("p"))
	for _, input := range []string{`.\31 23`, `[data-x="a\\b"]`, `#\-`} {
		if again := MustCompile(MustCompile(input).String()); !again.Match(p) {
			t.Errorf("%s: %q does not match", input, again.String())
		}
	}
}
