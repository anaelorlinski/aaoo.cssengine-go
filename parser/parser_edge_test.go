package parser

import (
	"strings"
	"testing"
	"time"
)

// P3: each failed declaration rebuilt the iterator by copying the rest of the
// input, which was quadratic in the number of nested rules.
func TestParseBlocksContentsManyNestedRules(t *testing.T) {
	const n = 20000
	tokens := Tokenize([]byte(strings.Repeat("b{} ", n)), false)
	start := time.Now()
	result := ParseBlocksContents(tokens, true)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %v", elapsed)
	}
	if len(result) != n {
		t.Fatalf("expected %d rules, got %d", n, len(result))
	}
	for _, r := range result {
		if _, ok := r.(QualifiedRule); !ok {
			t.Fatalf("expected a qualified rule, got %T", r)
		}
	}
}

// PC2: a byte order mark does not stick to the first selector.
func TestStylesheetBOM(t *testing.T) {
	rules := ParseStylesheetBytes([]byte("\xEF\xBB\xBFa{color:red}"), true, true)
	if len(rules) != 1 {
		t.Fatalf("%d rules", len(rules))
	}
	rule, ok := rules[0].(QualifiedRule)
	if !ok || Serialize(rule.Prelude) != "a" {
		t.Errorf("first rule %#v", rules[0])
	}
}

// PC11: !important is the last two significant tokens; custom properties
// may hold {} blocks; a declaration runs to the next ";".
func TestDeclarationBlocksAndImportant(t *testing.T) {
	decl := func(css string, nested bool) (Declaration, bool) {
		var list []Compound
		if nested {
			list = ParseBlocksContents(Tokenize([]byte(css), false), true)
		} else {
			list = ParseDeclarationList(Tokenize([]byte(css), false), true, true)
		}
		if len(list) == 0 {
			return Declaration{}, false
		}
		d, ok := list[0].(Declaration)
		return d, ok
	}
	for _, nested := range []bool{false, true} {
		for css, want := range map[string]string{
			"--x: a {b};": " a {b}",
			"--x: {a} b;": " {a} b",
			"color: {a};": " {a}",
			"--y : {} ;":  " {} ",
		} {
			d, ok := decl(css, nested)
			if !ok || Serialize(d.Value) != want {
				t.Errorf("%s (nested %v): %v %q, want %q", css, nested, ok, Serialize(d.Value), want)
			}
		}
		if _, ok := decl("color: {a} red;", nested); ok {
			t.Errorf("color: {a} red (nested %v) is a valid declaration", nested)
		}
		for css, want := range map[string]string{
			"a: b !important !important;": " b !important",
			"c: !!important;":             " !",
			"d: e ! important;":           " e",
			"e: f !IMPORTANT;":            " f",
			"f: important;":               "",
		} {
			d, ok := decl(css, nested)
			if got := strings.TrimSpace(Serialize(d.Value)); !ok || !d.Important && want != "" || got != strings.TrimSpace(want) && want != "" {
				t.Errorf("%s (nested %v): important %v, value %q, want %q", css, nested, d.Important, Serialize(d.Value), want)
			}
		}
	}
	// a nested rule after a declaration-like start is still a rule, and the
	// rest is read again
	list := ParseBlocksContents(Tokenize([]byte("a:hover { color: red } color: blue;"), false), true)
	if len(list) != 2 {
		t.Fatalf("%d items: %v", len(list), list)
	}
	if _, ok := list[0].(QualifiedRule); !ok {
		t.Errorf("first item %T", list[0])
	}
	if d, ok := list[1].(Declaration); !ok || d.Name != "color" {
		t.Errorf("second item %v", list[1])
	}
}

// P2-6: !important after a {} value is not another value (nested contents).
func TestNestedBlockValueImportant(t *testing.T) {
	list := ParseBlocksContents(Tokenize([]byte("a: {b} !important; c: d"), false), true)
	if len(list) != 2 {
		t.Fatalf("%d items: %v", len(list), list)
	}
	d, ok := list[0].(Declaration)
	if !ok || !d.Important || strings.TrimSpace(Serialize(d.Value)) != "{b}" {
		t.Errorf("first item %#v", list[0])
	}
	if d, ok := list[1].(Declaration); !ok || d.Name != "c" {
		t.Errorf("second item %v", list[1])
	}
}

// P2-7 to P2-9.
func TestSyntaxEdges2(t *testing.T) {
	tokens := RemoveWhitespace(Tokenize([]byte("1 -->"), true))
	if again := RemoveWhitespace(Tokenize([]byte(Serialize(tokens)), true)); len(again) != 2 {
		t.Errorf("1 --> serialized as %q", Serialize(tokens))
	}
	if _, bad := Tokenize([]byte("url(a\\\n)"), true)[0].(ParseError); !bad {
		t.Errorf("url(a\\<newline>) is a bad url")
	}
	rules := ParseStylesheetBytes([]byte("--x:hover {} a {}"), true, true)
	if len(rules) != 2 {
		t.Fatalf("%d rules", len(rules))
	}
	if _, isErr := rules[0].(ParseError); !isErr {
		t.Errorf("--x:hover {} is a rule: %v", rules[0])
	}
	if r, ok := rules[1].(QualifiedRule); !ok || strings.TrimSpace(Serialize(r.Prelude)) != "a" {
		t.Errorf("second rule %v", rules[1])
	}
}
