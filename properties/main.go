package properties

// There are 3 groups of types for a property, separated by 2 steps:
// cascading and computation. Thus the need of 3 types (see below).
// Schematically, the style computation is:
//
//	ValidatedProperty (ComputedFromCascaded)-> CascadedProperty (Compute)-> CssProperty

import (
	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
)

type Fl = utils.Fl

// DeclaredValue is the most general CSS input for a property,
// one of:
//   - the special "initial" or "inherited" keywords.
//   - a validated [CssProperty]
//   - a raw slice of tokens (containing var() tokens), pending validation
type DeclaredValue interface {
	isDeclaredValue()
}

func (DefaultValue) isDeclaredValue() {}
func (RawTokens) isDeclaredValue()    {}

type RawTokens []parser.Token

func (rt RawTokens) String() string {
	return parser.Serialize(rt)
}

// CssProperty is the final form of a css input, a.k.a. the computed value.
// Default values and "var()" have been resolved, and the raw stream of tokens has been
// validated.
type CssProperty interface {
	DeclaredValue

	isCssProperty()
}

type DefaultValue uint8

const (
	Inherit DefaultValue = iota + 1
	Initial
	// Revert and RevertLayer roll the cascade back to the previous origin,
	// or the previous cascade layer (CSS Cascade 5 §7.3.3, §7.3.4): the
	// cascade resolves them, they never reach computed values.
	Revert
	RevertLayer
)

func (d DefaultValue) String() string {
	switch d {
	case Inherit:
		return "<inherit>"
	case Initial:
		return "<initial>"
	case Revert:
		return "<revert>"
	case RevertLayer:
		return "<revert-layer>"
	default:
		return "invalid value"
	}
}

type VarData struct {
	Name    string // name of a custom property
	Default RawTokens
}

func (v VarData) IsNone() bool {
	return v.Name == "" && v.Default == nil
}

const NbProperties = nbProperties

func (p KnownProp) String() string {
	if int(p) < len(propsNames) {
		return propsNames[p]
	}
	if name, ok := propsNamesRegistered[p]; ok {
		return name
	}
	return "<unknown property>"
}

func (p KnownProp) Key() PropKey { return PropKey{KnownProp: p} }

// Properties is a general container for computed properties.
//
// In addition to the generic acces, an attempt to provide a "type safe" way is provided through the
// GetXXX and SetXXX methods. It relies on the convention than all the keys should be present,
// and values never be nil.
// Empty values are then encoded by the zero value of the concrete type.
type Properties map[KnownProp]CssProperty

// Copy returns a shallow copy: the map is new, the values are shared, and
// so must not be modified in place (see the package documentation).
func (p Properties) Copy() Properties {
	out := make(Properties, len(p))
	for name, v := range p {
		out[name] = v
	}
	return out
}

// UpdateWith merge the entries from `other` to `p`.
func (p Properties) UpdateWith(other Properties) {
	for k, v := range other {
		p[k] = v
	}
}

// PropKey stores a CSS property name, supporting variables.
type PropKey struct {
	Var string // with leading --
	KnownProp
}

func (pr PropKey) String() string {
	if pr.KnownProp != 0 {
		return pr.KnownProp.String()
	}
	return pr.Var
}

var _ StyleAccessor = Properties(nil)
