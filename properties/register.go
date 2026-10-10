package properties

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
)

// Setup-time registration of properties this package does not define.
//
// A consumer that owns a vocabulary the CSS engine has no business knowing —
// SVG's fill/stroke/marker-*, for one — registers it before any stylesheet is
// parsed. Registration allocates an enum value from a range reserved above
// the generated properties, so the built-in tables are untouched.
//
// # Concurrency
//
// Registration writes the tables (the names, the initial values, the known
// and inherited properties) that the rest of the engine reads without any
// lock. The registry is therefore a two-phase design:
//
//  1. Setup: [RegisterProperty] may be called, typically from a package
//     init function. Go runs init functions sequentially and they
//     happen-before main, so readers started afterwards see every write.
//  2. Sealed: after [Seal] (or the first [RegisteredProperty] lookup, which
//     seals implicitly), the tables are immutable and every read is
//     race-free. A late [RegisterProperty] panics instead of racing.
//
// (Used by the SVG engine, for the properties only SVG has.)

// firstRegisteredProp is the first enum value handed out by
// [RegisterProperty]. The generated properties stop below it.
const firstRegisteredProp = KnownProp(1024)

// lastRegisteredProp is the last enum value [RegisterProperty] may hand out.
const lastRegisteredProp = KnownProp(math.MaxUint16)

// Compile-time guard: the generated enum must stay below the registered
// range. If nbProperties ever grows past firstRegisteredProp, this constant
// expression overflows the unsigned KnownProp type and the build fails.
const _ = firstRegisteredProp - nbProperties

var (
	registerMu       sync.Mutex
	registeredByName = map[string]KnownProp{}
	// nextRegistered is the next free enum value; values in
	// [firstRegisteredProp, nextRegistered) are registered.
	nextRegistered atomic.Uint32
	sealed         atomic.Bool
)

func init() { nextRegistered.Store(uint32(firstRegisteredProp)) }

// RegisterProperty registers a property under `name`, returning its enum
// value. A property is registered once, by the package defining it.
//
// initial is the property's initial value, and inherited says whether it
// inherits. Must be called during setup (typically from an init function),
// before any stylesheet is parsed.
//
// It panics, since all of these are programming errors:
//   - when the registry is sealed (see [Seal]);
//   - when name is a property or shorthand built into the engine;
//   - when name is already registered;
//   - when the registered range is exhausted.
func RegisterProperty(name string, initial CssProperty, inherited bool) KnownProp {
	registerMu.Lock()
	defer registerMu.Unlock()

	if sealed.Load() {
		panic(fmt.Sprintf("properties: RegisterProperty(%q) after the registry was sealed; register during setup", name))
	}
	if _, ok := registeredByName[name]; ok {
		panic(fmt.Sprintf("properties: %q registered twice", name))
	}
	if _, builtin := propsFromNames[name]; builtin || NewShorthand(name) != 0 {
		panic(fmt.Sprintf("properties: cannot register %q: it is a built-in property", name))
	}
	next := nextRegistered.Load()
	if next > uint32(lastRegisteredProp) {
		panic("properties: registered property range exhausted")
	}
	p := KnownProp(next)

	registeredByName[name] = p
	propsNamesRegistered[p] = name
	propsFromNames[name] = p
	initialValues[p] = initial
	knownSet.Add(p)
	if inherited {
		inheritedSet.Add(p)
	}
	nextRegistered.Store(next + 1)
	return p
}

// Seal ends the setup phase: the property tables become immutable and any
// later [RegisterProperty] panics. It is idempotent and safe to call
// concurrently. Engine entry points should call it before parsing.
func Seal() {
	if sealed.Load() {
		return // fast path: called on every parse
	}
	registerMu.Lock()
	sealed.Store(true)
	registerMu.Unlock()
}

// IsSealed reports whether [Seal] has been called.
func IsSealed() bool { return sealed.Load() }

// Registering runs register, which registers what goes with a property
// registered by [RegisterProperty] (a validator, a computer), under the
// registry's lock: it panics, as RegisterProperty does, once the registry is
// sealed, and sealing waits for it. what names the caller, for the panic.
func Registering(what string, register func()) {
	registerMu.Lock()
	defer registerMu.Unlock()
	if sealed.Load() {
		panic(fmt.Sprintf("properties: %s after the registry was sealed; register during setup", what))
	}
	register()
}

// propsNamesRegistered names the registered properties, since the generated
// propsNames array cannot grow.
var propsNamesRegistered = map[KnownProp]string{}

// IsRegistered reports whether p came from [RegisterProperty].
func (p KnownProp) IsRegistered() bool {
	return p >= firstRegisteredProp && uint32(p) < nextRegistered.Load()
}

// RegisteredProperty looks up a registered property by name. The first
// lookup seals the registry (see [Seal]): a lookup means the engine is in
// use, and registering afterwards would race with readers.
func RegisteredProperty(name string) (KnownProp, bool) {
	if !sealed.Load() {
		Seal()
	}
	// sealed: registeredByName is immutable from now on
	p, ok := registeredByName[name]
	return p, ok
}
