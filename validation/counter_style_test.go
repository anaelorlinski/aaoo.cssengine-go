package validation

import (
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/counters"
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// VP5: the negative descriptor is the prefix, then the suffix.
func TestCounterStyleNegative(t *testing.T) {
	for decl, want := range map[string]string{
		`negative: "(" ")"`: "(12)",
		`negative: "-"`:     "-12",
		`negative: "~" "!"`: "~12!",
	} {
		desc := PreprocessCounterStyleDescriptors("", pa.ParseBlocksContents(
			pa.Tokenize([]byte(`system: numeric; symbols: "0" "1" "2"; `+decl), false), false))
		styles := counters.CounterStyle{"neg": desc}
		if got := styles.RenderValue(-5, "neg"); got != want {
			t.Errorf("%s: -5 renders %q, want %q", decl, got, want)
		}
	}
}

// VP12: registering a validator once stylesheets are parsed is refused (the
// validators are then read without synchronisation).
func TestRegisterValidatorAfterSeal(t *testing.T) {
	pr.Seal() // parsing seals the registry anyway
	defer func() {
		if recover() == nil {
			t.Error("RegisterValidator after Seal did not panic")
		}
	}()
	RegisterValidator(pr.KnownProp(2000), func([]Token, string) pr.CssProperty { return nil })
}

// C2-3: a var() value is supported by known properties only.
func TestSupportsVarUnknownProperty(t *testing.T) {
	for decl, want := range map[string]bool{
		"foo: var(--x)": false, "-webkit-foo: var(--x)": false,
		"color: var(--x)": true, "margin: var(--x) 1px": true, "margin-inline: var(--x)": true,
	} {
		d := pa.ParseDeclarationListString(decl, true, true)[0].(pa.Declaration)
		if got := Supports(d.Name, d.Value); got != want {
			t.Errorf("%s: %v, want %v", decl, got, want)
		}
	}
}

// V2-1: bleed rejects invalid values and accepts math.
func TestBleedValidation(t *testing.T) {
	for _, css := range []string{"bleed: foo", "bleed-top: foo", "bleed: 1px foo"} {
		if d := PreprocessDeclarations("", pa.ParseDeclarationListString(css, true, true)); len(d) != 0 {
			t.Errorf("%s accepted: %v", css, d)
		}
	}
	for _, css := range []string{"bleed: calc(1px)", "bleed: 3mm", "bleed: auto"} {
		if d := PreprocessDeclarations("", pa.ParseDeclarationListString(css, true, true)); len(d) == 0 {
			t.Errorf("%s rejected", css)
		}
	}
}

// M2-1: the validator of a built-in property cannot be replaced.
func TestRegisterValidatorBuiltin(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("RegisterValidator of a built-in property did not panic")
		}
	}()
	RegisterValidator(pr.PFontVariant, func([]Token, string) pr.CssProperty { return nil })
}
