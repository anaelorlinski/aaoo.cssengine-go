package properties

import "testing"

// PR1: inherited flags checked against the specs (and WeasyPrint's INHERITED).
func TestInheritedFlags(t *testing.T) {
	for _, tc := range []struct {
		p         KnownProp
		inherited bool
	}{
		{PBlockEllipsis, true},            // css-overflow-4: Inherited: yes
		{PImageOrientation, true},         // css-images-3: Inherited: yes
		{PTextDecorationThickness, false}, // css-text-decor-4: Inherited: no
		{PTextDecorationColor, false},
		{PColor, true},
		{PMarginTop, false},
	} {
		if got := inheritedSet.Has(tc.p); got != tc.inherited {
			t.Errorf("Inherited.Has(%s) = %v, want %v", tc.p, got, tc.inherited)
		}
	}
	// text-decoration-thickness is propagated like the other
	// text-decoration-* longhands instead.
	for _, p := range []KnownProp{PTextDecorationLine, PTextDecorationColor, PTextDecorationStyle, PTextDecorationThickness} {
		if !p.IsTextDecoration() {
			t.Errorf("%s should be a propagated text decoration", p)
		}
	}
	if PTextUnderlineOffset.IsTextDecoration() {
		t.Error("text-underline-offset is inherited, not propagated")
	}
}
