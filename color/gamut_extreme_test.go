package color

import (
	"math"
	"testing"
	"time"
)

// PC12: gamut mapping an OKLCH color with a huge chroma keeps its lightness
// (the XYZ round trip used to turn it black or white).
func TestGamutMapHugeChroma(t *testing.T) {
	reference, _ := Value{Space: OKLCH, Count: 3, Components: [4]float64{0.5, 1e15, 30}, Alpha: 1}.SRGB()
	for _, chroma := range []float64{1e15, 1e20, 3e38} {
		got, _ := Value{Space: OKLCH, Count: 3, Components: [4]float64{0.5, chroma, 30}, Alpha: 1}.SRGB()
		for i := range 3 {
			if math.Abs(got[i]-reference[i]) > 0.01 {
				t.Errorf("chroma %g: %v, want about %v", chroma, got, reference)
				break
			}
		}
	}
	// in the Oklab form too
	got, _ := Value{Space: OKLab, Count: 3, Components: [4]float64{0.5, 1e20, 0}, Alpha: 1}.SRGB()
	if got[0] < 0.1 || got[0] > 0.99 {
		t.Errorf("oklab(0.5 1e20 0): %v", got)
	}
}

// P2-1: an infinite chroma (a, b overflowing in hypot) does not hang the
// bisection.
func TestGamutMapInfiniteChroma(t *testing.T) {
	done := make(chan [3]float64)
	go func() {
		rgb, _ := Value{Space: OKLab, Count: 3, Components: [4]float64{0.5, 1.3e308, 1.3e308}, Alpha: 1}.SRGB()
		done <- rgb
	}()
	select {
	case rgb := <-done:
		for _, c := range rgb {
			if math.IsNaN(c) || c < 0 || c > 1 {
				t.Errorf("not in gamut: %v", rgb)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gamut mapping does not return")
	}
	for _, v := range []Value{
		{Space: OKLab, Count: 3, Components: [4]float64{0.5, math.Inf(1), 0}, Alpha: 1},
		{Space: OKLCH, Count: 3, Components: [4]float64{0.5, math.NaN(), 30}, Alpha: 1},
		{Space: OKLCH, Count: 3, Components: [4]float64{math.NaN(), 0.2, 30}, Alpha: 1},
	} {
		v.SRGB()
	}
}
