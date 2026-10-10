package validation

import (
	"testing"

	tu "github.com/anaelorlinski/aaoo.cssengine-go/internal/testutils"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// testViewport is a 300x200 page box with 50px margins, i.e. a 200x100
// page area — the same rectangle pair as WeasyPrint's test_units.
func testViewport() *pr.ViewportSizes {
	vs := pr.NewViewportSizes()
	vs.SetBox(300, 200)
	vs.SetArea(200, 100)
	return vs
}

// Every viewport unit name must parse, including the sv*/lv*/dv*
// aliases, which have no separate meaning in print.
func TestViewportUnitsParsed(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)

	for name, want := range map[string]pr.Unit{
		"vw": pr.Vw, "vh": pr.Vh, "vi": pr.Vi, "vb": pr.Vb, "vmin": pr.Vmin, "vmax": pr.Vmax,
		"svw": pr.Vw, "lvh": pr.Vh, "dvi": pr.Vi, "svb": pr.Vb, "lvmin": pr.Vmin, "dvmax": pr.Vmax,
		"pvw": pr.Pvw, "pvh": pr.Pvh, "pvi": pr.Pvi, "pvb": pr.Pvb, "pvmin": pr.Pvmin, "pvmax": pr.Pvmax,
	} {
		got, ok := lengthUnits[name]
		if !ok {
			t.Fatalf("%s is not a known length unit", name)
		}
		tu.AssertEqual(t, got, want)
		// The enum must round-trip through String(), which is used in
		// warnings and debug output.
		if name[0] == 'p' || name[0] == 'v' {
			tu.AssertEqual(t, got.String(), name)
		}
	}
}

// The two families must be distinguishable by range check alone.
func TestViewportUnitsClassification(t *testing.T) {
	for _, u := range []pr.Unit{pr.Vw, pr.Vh, pr.Vi, pr.Vb, pr.Vmin, pr.Vmax} {
		tu.AssertEqual(t, u.IsViewport(), true)
		tu.AssertEqual(t, u.IsPageBoxRelative(), false)
	}
	for _, u := range []pr.Unit{pr.Pvw, pr.Pvh, pr.Pvi, pr.Pvb, pr.Pvmin, pr.Pvmax} {
		tu.AssertEqual(t, u.IsViewport(), true)
		tu.AssertEqual(t, u.IsPageBoxRelative(), true)
	}
	for _, u := range []pr.Unit{pr.Px, pr.Em, pr.Perc, pr.Scalar, pr.Mm, pr.Fr, pr.Rad} {
		tu.AssertEqual(t, u.IsViewport(), false)
	}
}

func TestViewportSizesResolve(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)

	vs := testViewport()
	for _, data := range []struct {
		v    float32
		unit pr.Unit
		want float32
	}{
		// Page area: 200x100.
		{50, pr.Vh, 50}, {50, pr.Vw, 100}, {25, pr.Vb, 25}, {1.25, pr.Vi, 2.5},
		{10, pr.Vmin, 10}, {20, pr.Vmax, 40},
		// Page box: 300x200.
		{50, pr.Pvh, 100}, {50, pr.Pvw, 150}, {25, pr.Pvb, 50}, {1.5, pr.Pvi, 4.5},
		{10, pr.Pvmin, 20}, {20, pr.Pvmax, 60},
	} {
		got := vs.Resolve(data.v, data.unit)
		if !near(got, data.want, 0.001) {
			t.Fatalf("%g%s: expected %g, got %g", data.v, data.unit, data.want, got)
		}
	}
}

// Resolving before the rectangle is known warns and falls back to the
// initial A4 page rather than silently returning zero.
func TestViewportSizesFallback(t *testing.T) {
	capt := tu.CaptureLogs(t)
	empty := pr.NewViewportSizes()
	got := empty.Resolve(100, pr.Pvw)
	if !near(got, pr.InitialPageSize()[0], 0.001) {
		t.Fatalf("expected the A4 width %g, got %g", pr.InitialPageSize()[0], got)
	}
	if len(capt.Logs()) != 1 {
		t.Fatalf("expected one warning, got %v", capt.Logs())
	}

	// Knowing the page box does not make the page area known: the two
	// are published at different points of the page layout.
	capt = tu.CaptureLogs(t)
	boxOnly := pr.NewViewportSizes()
	boxOnly.SetBox(300, 200)
	tu.AssertEqual(t, near(boxOnly.Resolve(100, pr.Pvh), 200, 0.001), true)
	tu.AssertEqual(t, near(boxOnly.Resolve(100, pr.Vh), pr.InitialPageSize()[1], 0.001), true)
	if len(capt.Logs()) != 1 {
		t.Fatalf("expected one warning for the unknown page area, got %v", capt.Logs())
	}
}

// Viewport units inside math functions — the case themes actually hit.
func TestViewportUnitsInMath(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)

	ctx := MathContext{FontSize: 10, RootFontSize: 10, ViewportSizes: testViewport()}
	for _, data := range []struct {
		expr string
		want float32
	}{
		{"calc(50pvw)", 150},
		{"calc(50pvw - 50px)", 100},
		{"calc(10pvh * 2)", 40},
		{"min(50pvw, 100px)", 100},
		{"max(10vw, 10pvw)", 30},
		{"clamp(10px, 5pvh, 100px)", 10},
		{"calc(2em + 10vw)", 40},
	} {
		got := evalForTest(t, data.expr, pr.MathLengthPercentage, ctx)
		if !near(got, data.want, 0.01) {
			t.Fatalf("%s: expected %g, got %g", data.expr, data.want, got)
		}
	}
}

// A viewport unit inside `@page { size }` is the one genuine
// circularity, and must be rejected rather than resolved against an
// arbitrary base.
func TestViewportUnitsRejectedInSize(t *testing.T) {
	for _, decl := range []string{"50pvw", "50vh", "10pvw 20pvh", "300px 50pvh"} {
		tokens := parser.RemoveWhitespace(parser.Tokenize([]byte(decl), false))
		if got := size(tokens, ""); got != nil {
			t.Fatalf("size: %s should be rejected, got %v", decl, got)
		}
	}
	// Sanity check: ordinary lengths and keywords still validate.
	for _, decl := range []string{"a4", "300px 200px", "letter landscape"} {
		tokens := parser.RemoveWhitespace(parser.Tokenize([]byte(decl), false))
		if got := size(tokens, ""); got == nil {
			t.Fatalf("size: %s should be accepted", decl)
		}
	}
}
