package counters

import (
	"math"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

func ts(s string) pr.TaggedString { return pr.TaggedString{Tag: pr.String_, S: s} }

func syms(ss ...string) []pr.TaggedString {
	out := make([]pr.TaggedString, len(ss))
	for i, s := range ss {
		out[i] = ts(s)
	}
	return out
}

func decimalDesc() CounterStyleDescriptors {
	return CounterStyleDescriptors{
		System:  CounterStyleSystem{System: "numeric", Number: -1},
		Symbols: syms("0", "1", "2", "3", "4", "5", "6", "7", "8", "9"),
	}
}

// testStyles is a small stand-in for the UA stylesheet.
func testStyles() CounterStyle {
	return CounterStyle{
		"decimal": decimalDesc(),
		"lower-alpha": {
			System:  CounterStyleSystem{System: "alphabetic", Number: -1},
			Symbols: syms("a", "b", "c"),
		},
		"cyc": {
			System:  CounterStyleSystem{System: "cyclic", Number: -1},
			Symbols: syms("a", "b"),
		},
		"sym": {
			System:  CounterStyleSystem{System: "symbolic", Number: -1},
			Symbols: syms("*", "†"),
		},
		"roman": {
			System: CounterStyleSystem{System: "additive", Number: -1},
			AdditiveSymbols: []pr.IntNamedString{
				{Int: 10, TaggedString: ts("X")},
				{Int: 5, TaggedString: ts("V")},
				{Int: 1, TaggedString: ts("I")},
			},
		},
	}
}

func symbolsStyle(system string, s ...string) pr.CounterStyleID {
	return pr.CounterStyleID{Type: "symbols()", Name: system, Symbols: s}
}

// C1: a cyclic style with a negative value must wrap, not index -1.
func TestCyclicNegative(t *testing.T) {
	c := testStyles()
	for v, want := range map[int]string{-3: "a", -2: "b", -1: "a", 0: "b", 1: "a", 2: "b", 3: "a"} {
		if got := c.RenderValueStyle(v, symbolsStyle("cyclic", "a", "b")); got != want {
			t.Errorf("symbols(cyclic a b) %d = %q, want %q", v, got, want)
		}
		if got := c.RenderValue(v, "cyc"); got != want {
			t.Errorf("cyc %d = %q, want %q", v, got, want)
		}
	}
}

// C2: a weight-0 additive symbol (accepted by the validator) must not be
// used as a divisor.
func TestAdditiveZeroWeight(t *testing.T) {
	c := testStyles()
	c["z"] = CounterStyleDescriptors{
		System: CounterStyleSystem{System: "additive", Number: -1},
		AdditiveSymbols: []pr.IntNamedString{
			{Int: 5, TaggedString: ts("V")},
			{Int: 0, TaggedString: ts("z")},
			{Int: 1, TaggedString: ts("I")},
		},
	}
	for v, want := range map[int]string{0: "z", 3: "III", 7: "VII"} {
		if got := c.RenderValue(v, "z"); got != want {
			t.Errorf("%d = %q, want %q", v, got, want)
		}
	}
	// 3 cannot be made of V and z alone: fallback to decimal.
	c["z2"] = CounterStyleDescriptors{
		System: CounterStyleSystem{System: "additive", Number: -1},
		AdditiveSymbols: []pr.IntNamedString{
			{Int: 5, TaggedString: ts("V")},
			{Int: 0, TaggedString: ts("z")},
		},
	}
	if got := c.RenderValue(3, "z2"); got != "3" {
		t.Errorf("z2 3 = %q, want 3", got)
	}
}

// C3: the representation length is bounded; past 60 code points the
// fallback style is used (css-counter-styles-3 §3.1.4, §3.1.7).
func TestUnboundedRepresentation(t *testing.T) {
	c := testStyles()
	for _, v := range []int{1_000_000, math.MaxInt32, math.MaxInt64} {
		if got := c.RenderValue(v, "sym"); got != strconv.Itoa(v) {
			t.Errorf("sym %d = %.40q..., want decimal fallback", v, got)
		}
		if got := c.RenderValue(v, "roman"); got != strconv.Itoa(v) {
			t.Errorf("roman %d = %.40q..., want decimal fallback", v, got)
		}
		if got := c.RenderValueStyle(v, symbolsStyle("symbolic", "*")); got != strconv.Itoa(v) {
			t.Errorf("symbols(symbolic) %d = %.40q..., want decimal fallback", v, got)
		}
	}
	// Small values are unaffected.
	if got := c.RenderValue(3, "sym"); got != "**" {
		t.Errorf("sym 3 = %q", got)
	}
	if got := c.RenderValue(4, "sym"); got != "††" {
		t.Errorf("sym 4 = %q", got)
	}
	if got := c.RenderValue(28, "roman"); got != "XXVIII" {
		t.Errorf("roman 28 = %q", got)
	}
	// Exactly at the cap is still rendered.
	if got := c.RenderValueStyle(limits.CounterRepresentation, symbolsStyle("symbolic", "*")); utf8.RuneCountInString(got) != limits.CounterRepresentation {
		t.Errorf("at the cap: %q", got)
	}
	if got := c.RenderValueStyle(limits.CounterRepresentation+1, symbolsStyle("symbolic", "*")); got != strconv.Itoa(limits.CounterRepresentation+1) {
		t.Errorf("past the cap: %q", got)
	}

	// A huge pad does the same.
	c["padded"] = CounterStyleDescriptors{
		System:  CounterStyleSystem{Extends: "extends", System: "decimal"},
		Pad:     pr.IntNamedString{Int: math.MaxInt32, TaggedString: ts("0")},
		Symbols: nil,
	}
	if got := c.RenderValue(7, "padded"); got != "7" {
		t.Errorf("padded = %.40q..., want 7", got)
	}
	c["padded5"] = CounterStyleDescriptors{
		System: CounterStyleSystem{Extends: "extends", System: "decimal"},
		Pad:    pr.IntNamedString{Int: 5, TaggedString: ts("0")},
	}
	if got := c.RenderValue(7, "padded5"); got != "00007" {
		t.Errorf("padded5 = %q", got)
	}
	if got := c.RenderValue(-7, "padded5"); got != "-0007" {
		t.Errorf("padded5 -7 = %q", got)
	}
}

// C4: additive value 0 without a 0-weight symbol uses the fallback.
func TestAdditiveZeroValueFallback(t *testing.T) {
	c := testStyles()
	if got := c.RenderValue(0, "roman"); got != "0" {
		t.Errorf("roman 0 = %q, want 0", got)
	}
}

// C5: pad counts grapheme clusters, not bytes.
func TestPadCountsGraphemes(t *testing.T) {
	c := testStyles()
	c["cjk"] = CounterStyleDescriptors{
		System:  CounterStyleSystem{System: "cyclic", Number: -1},
		Symbols: syms("一"),
		Pad:     pr.IntNamedString{Int: 3, TaggedString: ts("0")},
	}
	if got := c.RenderValue(1, "cjk"); got != "00一" {
		t.Errorf("cjk = %q, want 00一", got)
	}
	// e + combining acute is a single cluster
	c["comb"] = CounterStyleDescriptors{
		System:  CounterStyleSystem{System: "cyclic", Number: -1},
		Symbols: syms("é"),
		Pad:     pr.IntNamedString{Int: 2, TaggedString: ts("0")},
	}
	if got := c.RenderValue(1, "comb"); got != "0é" {
		t.Errorf("comb = %q", got)
	}
	if n := graphemeCount("👍🏽a🇫🇷👨‍👩‍👧"); n != 4 {
		t.Errorf("graphemeCount = %d, want 4", n)
	}
}

// C6: fallback-cycle detection must not confuse a style named like a
// system keyword with that system.
func TestFallbackNamedLikeSystem(t *testing.T) {
	c := testStyles()
	c["cyclic"] = CounterStyleDescriptors{
		System:  CounterStyleSystem{System: "cyclic", Number: -1},
		Symbols: syms("z"),
	}
	c["fx"] = CounterStyleDescriptors{
		System:   CounterStyleSystem{System: "fixed", Number: 1},
		Symbols:  syms("a"),
		Fallback: "cyclic",
	}
	if got := c.RenderValue(5, "fx"); got != "z" {
		t.Errorf("fx 5 = %q, want z (fallback to the style named cyclic)", got)
	}
}

// Fallback chains that loop end in decimal instead of recursing forever.
func TestFallbackLoop(t *testing.T) {
	c := testStyles()
	c["a"] = CounterStyleDescriptors{
		System: CounterStyleSystem{System: "fixed", Number: 1}, Symbols: syms("A"), Fallback: "b",
	}
	c["b"] = CounterStyleDescriptors{
		System: CounterStyleSystem{System: "fixed", Number: 1}, Symbols: syms("B"), Fallback: "a",
	}
	if got := c.RenderValue(9, "a"); got != "9" {
		t.Errorf("a 9 = %q", got)
	}
	// decimal itself out of its range (values beyond int32)
	if got := c.RenderValue(math.MaxInt64, "decimal"); got != strconv.Itoa(math.MaxInt64) {
		t.Errorf("decimal max = %q", got)
	}
	if got := c.RenderValue(math.MinInt64, "lower-alpha"); got != strconv.Itoa(math.MinInt64) {
		t.Errorf("lower-alpha min = %q", got)
	}
}

// Explicit ranges may include values a system is not defined for.
func TestSymbolicAlphabeticBelowOne(t *testing.T) {
	c := testStyles()
	c["s0"] = CounterStyleDescriptors{
		System:  CounterStyleSystem{System: "symbolic", Number: -1},
		Symbols: syms("*"),
		Range:   pr.OptionalRanges{Ranges: [][2]int{{math.MinInt32, math.MaxInt32}}},
	}
	c["a0"] = CounterStyleDescriptors{
		System:  CounterStyleSystem{System: "alphabetic", Number: -1},
		Symbols: syms("a", "b"),
		Range:   pr.OptionalRanges{Ranges: [][2]int{{math.MinInt32, math.MaxInt32}}},
	}
	for _, name := range []string{"s0", "a0"} {
		if got := c.RenderValue(0, name); got != "0" {
			t.Errorf("%s 0 = %q, want 0", name, got)
		}
	}
	if got := c.RenderValue(-2, "a0"); got != "-b" {
		t.Errorf("a0 -2 = %q, want -b", got)
	}
}

// The sign is kept when a negative value falls back.
func TestNegativeFallbackKeepsSign(t *testing.T) {
	c := testStyles()
	c["sym1"] = CounterStyleDescriptors{
		System:  CounterStyleSystem{System: "symbolic", Number: -1},
		Symbols: syms("*"),
		Range:   pr.OptionalRanges{Ranges: [][2]int{{math.MinInt32, math.MaxInt32}}},
	}
	if got := c.RenderValue(-1000, "sym1"); got != "-1000" {
		t.Errorf("sym1 -1000 = %q, want -1000", got)
	}
}
