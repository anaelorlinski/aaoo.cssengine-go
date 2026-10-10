package properties

import (
	"strings"
	"sync"
	"testing"
)

// unsealed reopens the registry for the duration of a test. Tests in this
// package run sequentially, so toggling the global flag is safe here.
func unsealed(t *testing.T) {
	t.Helper()
	registerMu.Lock()
	prev := sealed.Load()
	sealed.Store(false)
	registerMu.Unlock()
	t.Cleanup(func() {
		registerMu.Lock()
		sealed.Store(prev)
		registerMu.Unlock()
	})
}

// mustPanic runs fn and returns the panic message, failing if fn returns.
func mustPanic(t *testing.T, fn func()) (msg string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic")
		}
		msg, _ = r.(string)
	}()
	fn()
	return ""
}

// TestRegisterProperty covers the mechanism aosvg needs for fill/stroke and
// the other 26 SVG properties absent from the generated enum
// (the SVG engine).
func TestRegisterProperty(t *testing.T) {
	unsealed(t)
	fill := RegisterProperty("fill", String("black"), true)

	if !fill.IsRegistered() {
		t.Fatalf("fill got a generated enum value %d", fill)
	}
	if fill < firstRegisteredProp {
		t.Fatalf("fill %d is below the reserved range %d", fill, firstRegisteredProp)
	}
	// It must not collide with anything generated.
	if fill < NbProperties {
		t.Fatalf("registered range overlaps the generated one (%d < %d)", fill, KnownProp(NbProperties))
	}

	// A property is registered once.
	mustPanic(t, func() { RegisterProperty("fill", String("black"), true) })

	// A non-inheriting one must not land in Inherited.
	stroke := RegisterProperty("stroke", String("none"), false)
	if inheritedSet.Has(stroke) {
		t.Fatal("stroke should not inherit")
	}
	if stroke == fill {
		t.Fatal("two properties got the same enum value")
	}

	// Reachable by name, as the validator looks it up.
	if got, ok := RegisteredProperty("fill"); !ok || got != fill {
		t.Fatalf("RegisteredProperty(fill) = %d, %v", got, ok)
	}
	if got := propsFromNames["fill"]; got != fill {
		t.Fatalf("PropertyByName(fill) = %d, want %d", got, fill)
	}

	// It must behave like a property: named, known, inheriting, with an
	// initial value.
	if fill.String() != "fill" {
		t.Fatalf("String() = %q, want fill", fill.String())
	}
	if !knownSet.Has(fill) {
		t.Fatal("fill is not a known property")
	}
	if !inheritedSet.Has(fill) {
		t.Fatal("fill should inherit")
	}
	if initialValues[fill] != String("black") {
		t.Fatalf("initial value = %v", initialValues[fill])
	}
}

// Properties is a map keyed by KnownProp, so a registered value must store
// and read back like any other.
func TestRegisteredPropertyStorage(t *testing.T) {
	unsealed(t)
	p := RegisterProperty("stroke-width", String("1px"), true)
	props := Properties{}
	props[p] = String("3px")
	if got := props[p]; got != String("3px") {
		t.Fatalf("stored %v, read back %v", String("3px"), got)
	}
}

// R1: a built-in name must not be hijacked.
func TestRegisterBuiltinNameRejected(t *testing.T) {
	unsealed(t)
	for _, name := range []string{"color", "font-size", "margin", "border-color"} {
		before := propsFromNames[name]
		msg := mustPanic(t, func() { RegisterProperty(name, String("x"), false) })
		if !strings.Contains(msg, "built-in") {
			t.Fatalf("%s: unexpected panic %q", name, msg)
		}
		if propsFromNames[name] != before {
			t.Fatalf("%s: the name table changed to %d", name, propsFromNames[name])
		}
	}
	if propsFromNames["color"] != PColor {
		t.Fatal("color was hijacked")
	}
}

// R3: registering a name twice is a programming error, whatever the
// arguments.
func TestRegisterTwicePanics(t *testing.T) {
	unsealed(t)
	RegisterProperty("x-test-twice", String("a"), true)
	if msg := mustPanic(t, func() { RegisterProperty("x-test-twice", String("a"), true) }); !strings.Contains(msg, "twice") {
		t.Fatalf("unexpected panic %q", msg)
	}
	mustPanic(t, func() { RegisterProperty("x-test-twice", Strings{"b"}, false) })
}

// R2: after sealing, registration panics instead of racing with readers.
func TestRegisterAfterSealPanics(t *testing.T) {
	unsealed(t)
	Seal()
	if !IsSealed() {
		t.Fatal("not sealed")
	}
	msg := mustPanic(t, func() { RegisterProperty("x-test-late", String("a"), false) })
	if !strings.Contains(msg, "sealed") {
		t.Fatalf("unexpected panic %q", msg)
	}
}

// R2: a lookup seals the registry implicitly.
func TestRegisteredPropertySeals(t *testing.T) {
	unsealed(t)
	RegisteredProperty("fill")
	if !IsSealed() {
		t.Fatal("RegisteredProperty should seal the registry")
	}
}

// R2: concurrent readers against a late writer; run with -race.
func TestRegistryConcurrentReads(t *testing.T) {
	unsealed(t)
	p := RegisterProperty("x-test-concurrent", String("a"), true)
	Seal()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if got, ok := RegisteredProperty("x-test-concurrent"); !ok || got != p {
					t.Error("lookup failed")
					return
				}
				_ = propsFromNames["x-test-concurrent"]
				_ = initialValues[p]
				_ = inheritedSet.Has(p)
				_ = p.IsRegistered()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = recover() }()
		RegisterProperty("x-test-concurrent-late", String("b"), false)
		t.Error("late registration should panic")
	}()
	wg.Wait()
}

// R4: only allocated values report as registered, and the range is wide.
func TestRegisteredRange(t *testing.T) {
	if NbProperties >= firstRegisteredProp {
		t.Fatalf("generated enum (%d) overlaps the registered range (%d)", NbProperties, firstRegisteredProp)
	}
	if KnownProp(lastRegisteredProp).IsRegistered() {
		t.Fatal("an unallocated value reports as registered")
	}
	if KnownProp(firstRegisteredProp - 1).IsRegistered() {
		t.Fatal("a value below the range reports as registered")
	}
	if n := int(lastRegisteredProp) - int(firstRegisteredProp) + 1; n < 1000 {
		t.Fatalf("only %d registrable slots", n)
	}
}
