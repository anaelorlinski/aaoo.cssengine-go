package properties

import "github.com/anaelorlinski/aaoo.cssengine-go/logger"

// Viewport-relative units in paged media.
//
// Ports WeasyPrint's `initial_page_sizes` (weasyprint/css/units.py) with
// one deliberate difference: the page *box* rectangle is published before
// the page margins are resolved, not after, so `@page { margin-top:
// 25pvh }` works instead of silently falling back. The staging is:
//
//	@page { size } ──► page box (W₀,H₀)      ← pv* resolves against this
//	                      │
//	                      ▼
//	        page margins (may use pv*)
//	                      │
//	                      ▼
//	              page area (W₁,H₁)          ← v* resolves against this
//
// The only genuine circularity — a viewport unit inside `@page { size }`
// itself — is rejected at validation time.

// InitialPageSize is the fallback rectangle (A4 portrait, in CSS pixels)
// used when a viewport unit is resolved before the first page is laid
// out. It mirrors WeasyPrint's INITIAL_PAGE_SIZE.
var initialPageSize = [2]Fl{
	Fl(210 * lengthsToPixels[Mm]),
	Fl(297 * lengthsToPixels[Mm]),
}

// ViewportSizes carries the two rectangles that viewport-relative units
// resolve against. One instance is shared by every style of a document
// (styles hold a pointer), and it is filled in as the first page is laid
// out — see [ViewportSizes.SetBox] and [ViewportSizes.SetArea].
//
// The zero value is usable: units resolved against it warn once per
// occurrence and fall back to [InitialPageSize].
type ViewportSizes struct {
	// BoxWidth, BoxHeight is the page box: the paper size from
	// `@page { size }`, margins included. pv* resolve against it.
	BoxWidth, BoxHeight Fl
	// AreaWidth, AreaHeight is the page area: what is left inside the
	// page margins, border and padding. v* resolve against it.
	AreaWidth, AreaHeight Fl

	hasBox, hasArea bool
}

// NewViewportSizes returns an empty, shareable holder.
func NewViewportSizes() *ViewportSizes { return new(ViewportSizes) }

// SetBox publishes the page box rectangle. Called once the first page's
// `size` is known, before its margins are resolved.
func (ps *ViewportSizes) SetBox(w, h Float) {
	ps.BoxWidth, ps.BoxHeight, ps.hasBox = Fl(w), Fl(h), true
}

// SetArea publishes the page area rectangle, once the first page's
// margins have been resolved.
func (ps *ViewportSizes) SetArea(w, h Float) {
	ps.AreaWidth, ps.AreaHeight, ps.hasArea = Fl(w), Fl(h), true
}

// rect returns the rectangle unit resolves against, and whether it is the
// real one (as opposed to the [InitialPageSize] fallback).
func (ps *ViewportSizes) rect(unit Unit) (w, h Fl, known bool) {
	if ps != nil {
		if unit.IsPageBoxRelative() {
			if ps.hasBox {
				return ps.BoxWidth, ps.BoxHeight, true
			}
		} else if ps.hasArea {
			return ps.AreaWidth, ps.AreaHeight, true
		}
	}
	return initialPageSize[0], initialPageSize[1], false
}

// Resolve converts `v<unit>` to CSS pixels. unit must satisfy
// [Unit.IsViewport]; anything else returns 0.
//
// Resolving before the relevant rectangle is known — a v* unit inside a
// page margin, say, which is circular by construction — warns and falls
// back to [InitialPageSize], matching WeasyPrint.
func (ps *ViewportSizes) Resolve(v Fl, unit Unit) Fl {
	if !unit.IsViewport() {
		return 0
	}
	w, h, known := ps.rect(unit)
	if !known {
		logger.L().Warn("viewport unit resolved before the first page layout, falling back to A4", "unit", unit)
	}
	var ref Fl
	// TODO: use writing-mode for vi and vb, which are the logical
	// inline/block axes. Assuming horizontal-tb, as WeasyPrint does.
	switch unit {
	case Vw, Vi, Pvw, Pvi:
		ref = w
	case Vh, Vb, Pvh, Pvb:
		ref = h
	case Vmin, Pvmin:
		ref = min(w, h)
	case Vmax, Pvmax:
		ref = max(w, h)
	}
	return v / 100 * ref
}
