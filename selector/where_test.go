package selector

import "testing"

func TestWherePseudoClass(t *testing.T) {
	sel, err := ParseGroup(":where(#a, .b) span")
	if err != nil {
		t.Fatalf("parse :where failed: %s", err)
	}
	// :where contributes zero: only the span type selector counts
	if got := sel[0].Specificity(); got != (Specificity{0, 0, 1}) {
		t.Errorf(":where specificity: got %v, want [0 0 1]", got)
	}
	isSel, err := ParseGroup(":is(#a, .b) span")
	if err != nil {
		t.Fatalf("parse :is failed: %s", err)
	}
	if got := isSel[0].Specificity(); got != (Specificity{1, 0, 1}) {
		t.Errorf(":is specificity: got %v, want [1 0 1]", got)
	}
}
