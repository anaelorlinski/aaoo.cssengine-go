package cascade

import (
	"reflect"
	"testing"
)

// The example of CSS Cascade 5 §6.4.3: reset.type, reset, framework.theme,
// framework, then the unlayered rules.
func TestLayerRanks(t *testing.T) {
	var l Layers
	resetType := l.Declare(0, []string{"reset", "type"})
	framework := l.Declare(0, []string{"framework"})
	theme := l.Declare(framework, []string{"theme"})
	reset := l.Declare(0, []string{"reset"})
	ranks := l.Ranks()
	order := []int{resetType, reset, theme, framework, 0}
	for i := 1; i < len(order); i++ {
		if ranks[order[i-1]] >= ranks[order[i]] {
			t.Errorf("rank of %v (%d) should be below %v (%d)",
				l.Name(order[i-1]), ranks[order[i-1]], l.Name(order[i]), ranks[order[i]])
		}
	}
	// declaring again returns the same layer
	if l.Declare(0, []string{"reset", "type"}) != resetType || l.Declare(0, []string{"framework", "theme"}) != theme {
		t.Error("redeclaration created a new layer")
	}
}

func TestLayerAnonymous(t *testing.T) {
	var l Layers
	a, b := l.Anonymous(0), l.Anonymous(0)
	if a == b {
		t.Fatal("anonymous layers must be distinct")
	}
	if ranks := l.Ranks(); ranks[a] >= ranks[b] {
		t.Error("anonymous layers are ordered by declaration")
	}
}

// Import merges the layers of stylesheets of one origin in document
// order: named layers merge, anonymous ones stay distinct.
func TestLayerImport(t *testing.T) {
	var first, second, doc Layers
	first.Declare(0, []string{"b"})
	firstAnon := first.Anonymous(0)
	first.Declare(0, []string{"a"})
	secondA := second.Declare(0, []string{"a"})
	secondAnon := second.Anonymous(0)
	secondC := second.Declare(0, []string{"c"})

	m1 := doc.Import(&first, 0)
	m2 := doc.Import(&second, 0)
	if m2[secondA] != doc.Declare(0, []string{"a"}) {
		t.Error("named layers of two stylesheets must merge")
	}
	if m2[secondAnon] == m1[firstAnon] {
		t.Error("anonymous layers of two stylesheets must stay distinct")
	}
	// first declarations: b, anon1, a (both sheets), anon2, c, then unlayered
	ranks := doc.Ranks()
	order := []int{doc.Declare(0, []string{"b"}), m1[firstAnon], doc.Declare(0, []string{"a"}), m2[secondAnon], m2[secondC], 0}
	for i := 1; i < len(order); i++ {
		if ranks[order[i-1]] >= ranks[order[i]] {
			t.Errorf("layer %v ranks above %v", doc.Name(order[i-1]), doc.Name(order[i]))
		}
	}
	if !reflect.DeepEqual(doc.Name(m2[secondC]), []string{"c"}) {
		t.Errorf("name: %v", doc.Name(m2[secondC]))
	}
}

// Reading a Layers (another's Import, Ranks, Len) must not modify it: the
// layers of a shared stylesheet are read by concurrent renders.
func TestLayersReadOnly(t *testing.T) {
	var shared Layers // zero value: only the outer layer
	done := make(chan bool)
	for range 4 {
		go func() {
			var doc Layers
			doc.Import(&shared, 0)
			_ = shared.Ranks()
			_ = shared.Len()
			done <- true
		}()
	}
	for range 4 {
		<-done
	}
	if shared.nodes != nil {
		t.Error("reading the zero Layers modified it")
	}
}
