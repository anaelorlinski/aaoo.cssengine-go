package cascade

import (
	"strconv"
	"testing"
	"time"
)

// Third review, SE3-7: declaring and importing many sibling layers is
// linear.
func TestManyLayers(t *testing.T) {
	start := time.Now()
	var l Layers
	for i := range 40_000 {
		l.Declare(0, []string{"l" + strconv.Itoa(i)})
	}
	for i := range 40_000 { // declared again: the same layers
		if id := l.Declare(0, []string{"l" + strconv.Itoa(i)}); id != i+1 {
			t.Fatalf("layer %d: id %d", i, id)
		}
	}
	var all Layers
	all.Import(&l, 0)
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %s", d)
	}
	if all.Len() != 40_001 {
		t.Errorf("%d layers", all.Len())
	}
}
