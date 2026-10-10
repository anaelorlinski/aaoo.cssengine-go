package validation

import (
	"testing"

	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// declNames validates the declaration block and returns the physical
// property names produced.
func declNames(t *testing.T, css string) []pr.KnownProp {
	t.Helper()
	decls := PreprocessDeclarations("", pa.ParseBlocksContents(pa.Tokenize([]byte(css), false), false))
	var names []pr.KnownProp
	for _, d := range decls {
		names = append(names, d.Name.KnownProp)
	}
	return names
}

func TestLogicalProperties(t *testing.T) {
	for _, test := range []struct {
		css  string
		want []pr.KnownProp
	}{
		{"margin-block-start: 1px", []pr.KnownProp{pr.PMarginTop}},
		{"margin-inline-end: 1px", []pr.KnownProp{pr.PMarginRight}},
		{"padding-inline-start: 1em", []pr.KnownProp{pr.PPaddingLeft}},
		{"inset-block-end: 0", []pr.KnownProp{pr.PBottom}},
		{"inline-size: 50%", []pr.KnownProp{pr.PWidth}},
		{"block-size: 10px", []pr.KnownProp{pr.PHeight}},
		{"min-inline-size: 1px", []pr.KnownProp{pr.PMinWidth}},
		{"max-block-size: 9px", []pr.KnownProp{pr.PMaxHeight}},
		{"margin-block: 1px", []pr.KnownProp{pr.PMarginTop, pr.PMarginBottom}},
		{"margin-inline: 1px 2px", []pr.KnownProp{pr.PMarginLeft, pr.PMarginRight}},
		{"padding-block: 1px 2px", []pr.KnownProp{pr.PPaddingTop, pr.PPaddingBottom}},
		{"inset-inline: 0 5px", []pr.KnownProp{pr.PLeft, pr.PRight}},
		{"inset: 1px 2px 3px 4px", []pr.KnownProp{pr.PTop, pr.PRight, pr.PBottom, pr.PLeft}},
		{"inset: 1px", []pr.KnownProp{pr.PTop, pr.PRight, pr.PBottom, pr.PLeft}},
		{
			"border-block: 1px solid red",
			[]pr.KnownProp{
				pr.PBorderTopWidth, pr.PBorderTopColor, pr.PBorderTopStyle,
				pr.PBorderBottomWidth, pr.PBorderBottomColor, pr.PBorderBottomStyle,
			},
		},
		{"border-inline-start-color: red", []pr.KnownProp{pr.PBorderLeftColor}},
		{"border-start-start-radius: 4px", []pr.KnownProp{pr.PBorderTopLeftRadius}},
		// invalid value counts are dropped
		{"margin-block: 1px 2px 3px", nil},
		{"inset: 1px 2px 3px 4px 5px", nil},
	} {
		got := declNames(t, test.css)
		if len(got) != len(test.want) {
			t.Errorf("%q: got %v, want %v", test.css, got, test.want)
			continue
		}
		for i := range got {
			if got[i] != test.want[i] {
				t.Errorf("%q: got %v, want %v", test.css, got, test.want)
				break
			}
		}
	}
}
