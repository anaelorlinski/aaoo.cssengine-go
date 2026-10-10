package counters

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

var fuzzSystems = []string{"cyclic", "fixed", "symbolic", "alphabetic", "numeric", "additive"}

// FuzzRenderValue renders arbitrary values with arbitrary (possibly
// degenerate) counter styles: it must never panic, hang or produce an
// unbounded string.
func FuzzRenderValue(f *testing.F) {
	f.Add(-2, uint8(0), "a b", 0, int64(0), int64(0), uint8(0), "", false)
	f.Add(3, uint8(5), "5 V,0 z", 0, int64(0), int64(0), uint8(0), "", false)
	f.Add(2147483647, uint8(2), "*", 0, int64(0), int64(0), uint8(0), "", false)
	f.Add(7, uint8(4), "0 1 2", 1000000, int64(-5), int64(5), uint8(1), "一", true)
	f.Add(-9223372036854775808, uint8(3), "a b", 0, int64(-9223372036854775808), int64(9223372036854775807), uint8(2), "x", false)
	f.Fuzz(func(t *testing.T, value int, sysIdx uint8, symbols string, pad int, rlo, rhi int64, fallbackIdx uint8, padSym string, extends bool) {
		system := fuzzSystems[int(sysIdx)%len(fuzzSystems)]
		desc := CounterStyleDescriptors{
			System:   CounterStyleSystem{System: system, Number: int(rlo % 10)},
			Pad:      pr.IntNamedString{Int: pad, TaggedString: ts(padSym)},
			Fallback: []string{"", "decimal", "fz", "other", "cyclic"}[int(fallbackIdx)%5],
		}
		if extends {
			desc.System = CounterStyleSystem{Extends: "extends", System: []string{"decimal", "fz", "other", "missing"}[int(sysIdx)%4]}
		}
		if rlo <= rhi && rlo != 0 {
			desc.Range = pr.OptionalRanges{Ranges: [][2]int{{int(rlo), int(rhi)}}}
		} else {
			desc.Range.Auto = true
		}
		for i, s := range strings.Split(symbols, ",") {
			if system == "additive" {
				var w int
				fields := strings.Fields(s)
				if len(fields) > 0 {
					for _, r := range fields[0] {
						w = w*10 + int(r)%10
					}
					s = strings.Join(fields[1:], " ")
				}
				desc.AdditiveSymbols = append(desc.AdditiveSymbols, pr.IntNamedString{Int: w - i, TaggedString: ts(s)})
			} else {
				for _, sym := range strings.Fields(s) {
					desc.Symbols = append(desc.Symbols, ts(sym))
				}
			}
		}
		c := testStyles()
		c["fz"] = desc
		c["other"] = CounterStyleDescriptors{
			System:   CounterStyleSystem{System: "fixed", Number: 1},
			Symbols:  syms("o"),
			Fallback: "fz",
		}
		check := func(out string) {
			// symbols themselves may be long; what must not happen is
			// growth with the value or the pad (alphabetic and numeric are
			// logarithmic: at most 64 digits)
			limit := 4*limits.CounterRepresentation + 64 + 64*len(symbols) + 4*len(padSym)
			if utf8.RuneCountInString(out) > limit {
				t.Fatalf("output too long: %d runes", utf8.RuneCountInString(out))
			}
		}
		check(c.RenderValue(value, "fz"))
		check(c.RenderMarker(pr.CounterStyleID{Name: "fz"}, value))
		check(c.RenderValueStyle(value, pr.CounterStyleID{Type: "symbols()", Name: system, Symbols: strings.Fields(symbols)}))
	})
}
