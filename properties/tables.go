package properties

import (
	"slices"
	"sort"
)

// The tables describing the properties are read through the functions
// below: they are shared by every document, and must not be modified.
// (Registration, [RegisterProperty], is the only writer, at setup.)

// InitialValue returns the initial value of p, or nil for an unknown
// property. The value is shared: it must not be modified.
func InitialValue(p KnownProp) CssProperty { return initialValues[p] }

// InitialAccessor reads the initial values with typed getters, as
// [Properties] does: InitialStyle.GetFontSize().
type InitialAccessor struct{}

// InitialStyle is the read-only view of the initial values.
var InitialStyle InitialAccessor

// CopyInitialValues returns a new map of the initial values, to be
// modified. The values themselves are shared, as by [Properties.Copy]:
// replace them, do not modify them.
func CopyInitialValues() Properties { return initialValues.Copy() }

// IsInherited reports whether p is an inherited property.
func IsInherited(p KnownProp) bool { return inheritedSet.Has(p) }

// PropertyByName returns the property named name (built in or
// registered), or 0 if there is none.
func PropertyByName(name string) KnownProp { return propsFromNames[name] }

// IsKnownProperty reports whether p is a property of the engine (built in
// or registered).
func IsKnownProperty(p KnownProp) bool { return knownSet.Has(p) }

// KnownProperties returns the properties of the engine, in enum order.
func KnownProperties() []KnownProp { return sortedProps(knownSet) }

// TableWrapperProperties returns the properties that apply to the wrapper
// box of a table rather than to the table box (CSS 2.1 §17.4), in enum
// order.
func TableWrapperProperties() []KnownProp { return sortedProps(tableWrapperSet) }

// IsInitialNotComputed reports whether the initial value of p is not its
// computed value, and must go through the computer.
func IsInitialNotComputed(p KnownProp) bool { return initialNotComputedSet.Has(p) }

// PageSize returns the size of the named page (a4, letter…, lowercase),
// in portrait orientation.
func PageSize(name string) (Point, bool) {
	size, ok := pageSizes[name]
	return size, ok
}

// PageSizeNames returns the names of the page sizes, sorted.
func PageSizeNames() []string {
	names := make([]string, 0, len(pageSizes))
	for name := range pageSizes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// A4 returns the A4 page size, in portrait orientation.
func A4() Point { return a4 }

// InitialPageSize returns the rectangle, in CSS pixels, viewport units
// resolve against before the first page is laid out (A4 portrait).
func InitialPageSize() [2]Fl { return initialPageSize }

// LengthToPixels returns the number of CSS pixels in one unit u, or 0 if
// u is not an absolute length unit.
func LengthToPixels(u Unit) Float { return lengthsToPixels[u] }

// FontSizeKeywords returns the font sizes of the keywords xx-small to
// xx-large, in pixels.
func FontSizeKeywords() [7]Float { return fontSizeKeywords }

func sortedProps(s SetK) []KnownProp {
	out := make([]KnownProp, 0, len(s))
	for p := range s {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}
