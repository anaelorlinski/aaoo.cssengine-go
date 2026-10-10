package cascade

import (
	"reflect"
	"testing"

	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
)

func tokens(s string) []pa.Token { return pa.Tokenize([]byte(s), true) }

func TestParseLayerName(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"a", []string{"a"}},
		{" framework.theme ", []string{"framework", "theme"}},
		{"a . b", nil}, // no whitespace inside
		{"a.", nil},
		{".a", nil},
		{"", nil},
		{"initial", nil}, // CSS-wide keywords are reserved
		{"a.Revert", nil},
		{"1a", nil},
	} {
		got, ok := ParseLayerName(tokens(tc.in))
		if ok != (tc.want != nil) || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v %v, want %v", tc.in, got, ok, tc.want)
		}
	}
}

func TestParseLayerRule(t *testing.T) {
	var l Layers
	stmt, ok := ParseLayerRule(tokens("a, b.c"), false)
	if !ok || stmt.Declare(&l, 0) != 0 {
		t.Fatal("a statement declares layers and keeps the current one")
	}
	if l.Len() != 4 { // outer, a, b, b.c
		t.Errorf("%d layers", l.Len())
	}
	block, _ := ParseLayerRule(tokens("b.c"), true)
	if id := block.Declare(&l, 0); !reflect.DeepEqual(l.Name(id), []string{"b", "c"}) {
		t.Errorf("block layer %v", l.Name(id))
	}
	anon, _ := ParseLayerRule(tokens(" "), true)
	if a, b := anon.Declare(&l, 0), anon.Declare(&l, 0); a == b {
		t.Error("anonymous blocks must declare distinct layers")
	}
	if _, ok := ParseLayerRule(tokens("a,"), false); ok {
		t.Error("a trailing comma is invalid")
	}
	if _, ok := ParseLayerRule(tokens(""), false); ok {
		t.Error("an empty statement is invalid")
	}
}

func TestParseImportLayer(t *testing.T) {
	_, has, rest, ok := ParseImportLayer(pa.RemoveWhitespace(tokens("layer print")))
	if !has || !ok || len(rest) != 1 {
		t.Errorf("layer keyword: %v %v %v", has, ok, rest)
	}
	rule, has, _, ok := ParseImportLayer(pa.RemoveWhitespace(tokens("layer(a.b)")))
	if !has || !ok || !reflect.DeepEqual(rule.Names, [][]string{{"a", "b"}}) {
		t.Errorf("layer(): %v %v %v", rule, has, ok)
	}
	if _, _, _, ok := ParseImportLayer(pa.RemoveWhitespace(tokens("layer(a b)"))); ok {
		t.Error("an invalid layer() makes the @import invalid")
	}
	if _, has, rest, ok := ParseImportLayer(pa.RemoveWhitespace(tokens("screen"))); has || !ok || len(rest) != 1 {
		t.Error("no layer")
	}
}

func TestSortByLayer(t *testing.T) {
	ranks := []int{2, 0, 1} // outer, a, b: a < b < outer
	ms := []Matched[decl]{{Layer: 0, Order: 0}, {Layer: 1, Order: 1}, {Layer: 2, Order: 2}, {Layer: 1, Order: 3}}
	normal := append([]Matched[decl](nil), ms...)
	SortByLayer(normal, ranks, false)
	if got := orders(normal); !reflect.DeepEqual(got, []int{1, 3, 2, 0}) {
		t.Errorf("normal: %v", got)
	}
	important := append([]Matched[decl](nil), ms...)
	SortByLayer(important, ranks, true)
	if got := orders(important); !reflect.DeepEqual(got, []int{0, 2, 1, 3}) {
		t.Errorf("important: %v", got)
	}
}

func orders(ms []Matched[decl]) (out []int) {
	for _, m := range ms {
		out = append(out, m.Order)
	}
	return out
}
