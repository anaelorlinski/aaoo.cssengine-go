package validation

import (
	"testing"

	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// hyphenate-limit-chars: [ auto | <integer> ]{1,3} (CSS Text 4). auto is
// kept, for the hyphenation to choose with the language; a missing third
// value is the second one, a missing second value is auto.
func TestHyphenateLimitChars(t *testing.T) {
	const auto = pr.LimitAuto
	for css, want := range map[string]pr.Limits{
		"auto":           {Total: auto, Left: auto, Right: auto},
		"6":              {Total: 6, Left: auto, Right: auto},
		"6 3":            {Total: 6, Left: 3, Right: 3},
		"6 3 2":          {Total: 6, Left: 3, Right: 2},
		"5 2 2":          {Total: 5, Left: 2, Right: 2},
		"auto 3":         {Total: auto, Left: 3, Right: 3},
		"auto auto 4":    {Total: auto, Left: auto, Right: 4},
		"6 auto":         {Total: 6, Left: auto, Right: auto},
		"auto auto auto": {Total: auto, Left: auto, Right: auto},
	} {
		d := expandToDict(t, "hyphenate-limit-chars: "+css, "")
		if got := d[pr.PHyphenateLimitChars]; got != want {
			t.Errorf("hyphenate-limit-chars: %s -> %v, want %v", css, got, want)
		}
	}
	for _, css := range []string{"-1", "5 -2", "2.5", "5 2 2 2", "none", "5px"} {
		assertInvalid(t, "hyphenate-limit-chars: "+css, "invalid")
	}
	if got := pr.InitialValue(pr.PHyphenateLimitChars); got != (pr.Limits{Total: auto, Left: auto, Right: auto}) {
		t.Errorf("initial value %v, want auto", got)
	}
}
