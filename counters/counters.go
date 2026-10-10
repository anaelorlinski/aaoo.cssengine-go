package counters

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// Implement the various counter types and list-style-type values.
//
// These are defined in the same terms as CSS 3 Lists:
// http://drafts.csswg.org/csswg/css3-lists/#predefined-counters

// CounterStyle holds the counter styles of a document by name: the
// predefined ones and those of its @counter-style rules. It renders
// counter values (see [CounterStyle.RenderValue]).
type CounterStyle map[string]CounterStyleDescriptors

// may return nil
func (c CounterStyle) resolveCounter(counterName string, previousTypes utils.Set) *CounterStyleDescriptors {
	counter, has := c[counterName]
	if !has {
		return nil
	}

	// Avoid circular fallbacks
	if previousTypes == nil {
		previousTypes = utils.NewSet()
	} else if previousTypes.Has(counterName) {
		return nil
	}
	previousTypes.Add(counterName)

	extends, system := "", "symbolic"
	if counter.System != (CounterStyleSystem{}) {
		extends, system = counter.System.Extends, counter.System.System
	}

	// Handle extends
	for extends != "" {
		if extendedCounter, has := c[system]; has {
			counter.System = extendedCounter.System
			previousTypes.Add(system)

			extends, system = "", "symbolic"
			if counter.System != (CounterStyleSystem{}) {
				extends, system = counter.System.Extends, counter.System.System
			}

			if extends != "" && previousTypes.Has(system) {
				extends, system = "extends", "decimal"
				continue
			}
			counter.merge(extendedCounter)
		} else {
			return &counter
		}
	}

	return &counter
}

func (c CounterStyle) resolveCounterStyle(counterStyle pr.CounterStyleID, previousTypes utils.Set) *CounterStyleDescriptors {
	if counterType := counterStyle.Type; counterType == "symbols()" || counterType == "string" {
		var out CounterStyleDescriptors
		if counterType == "string" {
			out.System = CounterStyleSystem{"", "cyclic", -1}
			out.Symbols = []pr.TaggedString{{Tag: pr.String_, S: counterStyle.Name}}
			out.Suffix = pr.TaggedString{Tag: pr.String_, S: ""}
		} else if counterType == "symbols()" {
			out.System = CounterStyleSystem{"", counterStyle.Name, -1}
			if counterStyle.Name == "fixed" {
				out.System.Number = 1
			}
			for _, argument := range counterStyle.Symbols {
				out.Symbols = append(out.Symbols, pr.TaggedString{Tag: pr.String_, S: argument})
			}
			out.Suffix = pr.TaggedString{Tag: pr.String_, S: " "}
		}
		out.Negative = [2]pr.TaggedString{{Tag: pr.String_, S: "-"}, {Tag: pr.String_, S: ""}}
		out.Prefix = pr.TaggedString{Tag: pr.String_, S: ""}
		out.Range.Auto = true
		out.Fallback = "decimal"
		return &out
	}

	counterName := counterStyle.Name
	return c.resolveCounter(counterName, previousTypes)
}

// RenderValue generates the representation of counterValue in the counter
// style named counterStyleName.
//
// See https://www.w3.org/TR/css-counter-styles-3/#generate-a-counter
func (c CounterStyle) RenderValue(counterValue int, counterStyleName string) string {
	return c.renderValue(counterValue, c.resolveCounter(counterStyleName, nil), nil)
}

// RenderValueStyle is the same as `RenderValue`, for a general counter style.
func (c CounterStyle) RenderValueStyle(counterValue int, counterStyle pr.CounterStyleID) string {
	return c.renderValue(counterValue, c.resolveCounterStyle(counterStyle, nil), nil)
}

// decimal renders the value with the decimal style, the last resort of every
// fallback chain. decimal cannot be overridden (css-counter-styles-3 §6.1),
// so it is computed directly rather than looked up, which also guarantees
// termination for values outside decimal's own range.
func (c CounterStyle) decimal(counterValue int) string {
	if _, has := c["decimal"]; !has {
		// Could happen if the UA stylesheet is not used
		return ""
	}
	return strconv.Itoa(counterValue)
}

// renderFallback renders the value with the fallback style of counter.
// visited holds the names of the fallback styles already tried; a loop
// ends in decimal.
func (c CounterStyle) renderFallback(counterValue int, counter *CounterStyleDescriptors, visited utils.Set) string {
	name := counter.fallback()
	if visited.Has(name) {
		return c.decimal(counterValue)
	}
	visited.Add(name)
	return c.renderValue(counterValue, c.resolveCounter(name, nil), visited)
}

// renderValue implements the "generate a counter representation" algorithm.
// visited holds the names of the fallback styles used so far (nil at the
// top level).
func (c CounterStyle) renderValue(counterValue int, counter *CounterStyleDescriptors, visited utils.Set) string {
	if counter == nil {
		return c.decimal(counterValue)
	}
	if visited == nil {
		visited = utils.NewSet()
	}

	extends, system, fixedNumber := "", "symbolic", -1
	if counter.System != (CounterStyleSystem{}) {
		extends, system, fixedNumber = counter.System.Extends, counter.System.System, counter.System.Number
	}

	// Handle extends. While extends is set, system names the extended
	// counter style; seen guards against extends loops.
	seen := utils.NewSet()
	for extends != "" {
		if seen.Has(system) {
			return c.decimal(counterValue)
		}
		seen.Add(system)
		extendedCounter, has := c[system]
		if !has {
			return c.decimal(counterValue)
		}
		counter.System = extendedCounter.System
		extends, system, fixedNumber = "", "symbolic", -1
		if counter.System != (CounterStyleSystem{}) {
			extends, system, fixedNumber = counter.System.Extends, counter.System.System, counter.System.Number
		}
		counter.merge(extendedCounter)
	}

	// Step 2
	counterRanges := counter.Range.Ranges
	if counter.Range.Auto || counter.Range.IsNone() {
		minRange, maxRange := math.MinInt32, math.MaxInt32
		if system == "alphabetic" || system == "symbolic" {
			minRange = 1
		} else if system == "additive" {
			minRange = 0
		}
		counterRanges = [][2]int{{minRange, maxRange}}
	}
	var found bool
	for _, v := range counterRanges {
		if v[0] <= counterValue && counterValue <= v[1] {
			found = true
			break
		}
	}

	if !found {
		return c.renderFallback(counterValue, counter, visited)
	}

	// Step 3
	var (
		negativePrefix, negativeSuffix string
		useNegative                    bool
	)
	value := counterValue
	isNegative := value < 0
	if isNegative {
		vs := counter.Negative
		if vs == ([2]pr.TaggedString{}) {
			vs = [2]pr.TaggedString{{Tag: pr.String_, S: "-"}, {Tag: pr.String_, S: ""}}
		}
		negativePrefix, negativeSuffix = symbol(vs[0]), symbol(vs[1])
		useNegative = system == "symbolic" || system == "alphabetic" || system == "numeric" || system == "additive"
		if useNegative {
			if value == math.MinInt {
				// -value overflows
				return c.renderFallback(counterValue, counter, visited)
			}
			value = -value
		}
	}

	var (
		initial string
		ok      bool
	)
	switch system {
	case "cyclic":
		initial, ok = repeating(counter.Symbols, value)
	case "fixed":
		initial, ok = nonRepeating(counter.Symbols, fixedNumber, value)
	case "symbolic":
		initial, ok = symbolic(counter.Symbols, value)
	case "alphabetic":
		initial, ok = alphabetic(counter.Symbols, value)
	case "numeric":
		initial, ok = numeric(counter.Symbols, value)
	case "additive":
		initial, ok = additive(counter.AdditiveSymbols, value)
	}
	if !ok {
		// The algorithm cannot represent the value (or the representation
		// would be too long): use the fallback style.
		return c.renderFallback(counterValue, counter, visited)
	}

	// Step 4: pad, counting grapheme clusters (css-counter-styles-3 §4.6)
	pad := counter.Pad
	if pad.Int > 0 {
		padDifference := pad.Int - graphemeCount(initial)
		if isNegative && useNegative {
			padDifference -= graphemeCount(negativePrefix) + graphemeCount(negativeSuffix)
		}
		if padDifference > 0 {
			padSymbol := symbol(pad.TaggedString)
			if n := graphemeCount(padSymbol); n > 0 {
				if padDifference > (limits.CounterRepresentation-graphemeCount(initial))/n {
					return c.renderFallback(counterValue, counter, visited)
				}
				initial = strings.Repeat(padSymbol, padDifference) + initial
			}
		}
	}

	// Step 5
	if isNegative && useNegative {
		initial = negativePrefix + initial + negativeSuffix
	}

	// Step 6
	return initial
}

func symbol(value pr.TaggedString) string {
	if value.Tag == pr.String_ {
		return value.S
	}
	return ""
}

// graphemeCount approximates the number of extended grapheme clusters of s
// (UAX #29): combining marks, variation selectors, emoji modifiers and the
// rune following a zero width joiner extend the previous cluster, and
// regional indicators pair up.
func graphemeCount(s string) int {
	n := 0
	afterZWJ, pendingRI := false, false
	for _, r := range s {
		switch {
		case r == '‍':
			afterZWJ = true
			continue
		case afterZWJ:
			afterZWJ = false
			continue
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc),
			0x1F3FB <= r && r <= 0x1F3FF: // emoji modifiers
			if n > 0 {
				continue
			}
		case 0x1F1E6 <= r && r <= 0x1F1FF: // regional indicators
			if pendingRI {
				pendingRI = false
				continue
			}
			pendingRI = true
			n++
			continue
		}
		pendingRI = false
		n++
	}
	return n
}

// Implement the algorithm for `type: repeating`.
func repeating(symbols []pr.TaggedString, value int) (string, bool) {
	L := len(symbols)
	if L == 0 {
		return "", false
	}
	return symbol(symbols[((value-1)%L+L)%L]), true
}

// Implement the algorithm for `type: non-repeating`.
func nonRepeating(symbols []pr.TaggedString, firstValue, value int) (string, bool) {
	L := len(symbols)
	if value < firstValue {
		return "", false
	}
	value -= firstValue
	if 0 <= value && value < L {
		return symbol(symbols[value]), true
	}
	return "", false
}

// Implement the algorithm for `type: symbolic`, defined for value >= 1.
func symbolic(symbols []pr.TaggedString, value int) (string, bool) {
	L := len(symbols)
	if L == 0 || value < 1 {
		return "", false
	}
	sym := symbol(symbols[(value-1)%L])
	repeat := (value-1)/L + 1
	if n := graphemeCount(sym); n > 0 && repeat > limits.CounterRepresentation/n {
		return "", false
	}
	return strings.Repeat(sym, repeat), true
}

// Implement the algorithm for `type: alphabetic`, defined for value >= 1.
func alphabetic(symbols []pr.TaggedString, value int) (string, bool) {
	L := len(symbols)
	if L < 2 || value < 1 {
		return "", false
	}
	reversedParts := []string{}
	for value != 0 {
		value -= 1
		reversedParts = append(reversedParts, symbol(symbols[value%L]))
		value /= L
	}
	reverse(reversedParts)
	return strings.Join(reversedParts, ""), true
}

// Implement the algorithm for `type: numeric`.
func numeric(symbols []pr.TaggedString, value int) (string, bool) {
	L := len(symbols)
	if L < 2 || value < 0 {
		return "", false
	}
	if value == 0 {
		return symbol(symbols[0]), true
	}
	var reversedParts []string
	for value != 0 {
		reversedParts = append(reversedParts, symbol(symbols[value%L]))
		value /= L
	}
	reverse(reversedParts)
	return strings.Join(reversedParts, ""), true
}

// Implement the algorithm for `type: additive`.
func additive(symbols []pr.IntNamedString, value int) (string, bool) {
	if value < 0 {
		return "", false
	}
	if value == 0 {
		for _, vs := range symbols {
			if vs.Int == 0 {
				return symbol(vs.TaggedString), true
			}
		}
		// No 0-weight symbol: the value cannot be represented.
		return "", false
	}
	var (
		parts  []string
		length int
	)
	for _, vs := range symbols {
		if vs.Int <= 0 {
			continue // a 0 weight only represents the value 0
		}
		repetitions := value / vs.Int
		if repetitions == 0 {
			continue
		}
		sym := symbol(vs.TaggedString)
		if n := graphemeCount(sym); n > 0 {
			if repetitions > (limits.CounterRepresentation-length)/n {
				return "", false // too long
			}
			length += repetitions * n
		}
		parts = append(parts, strings.Repeat(sym, repetitions))
		value -= vs.Int * repetitions
		if value == 0 {
			return strings.Join(parts, ""), true
		}
	}
	return "", false //  Failed to find a representation for this value
}

// RenderMarker generates the content of a ::marker pseudo-element, for the
// given value.
func (c CounterStyle) RenderMarker(counterName pr.CounterStyleID, counterValue int) string {
	counter := c.resolveCounterStyle(counterName, nil)
	if counter == nil {
		if _, has := c["decimal"]; has {
			return c.RenderMarker(pr.CounterStyleID{Name: "decimal"}, counterValue)
		}
		// Could happen if the UA stylesheet is ! used
		return ""
	}

	prefix := symbol(counter.Prefix)
	suffix := counter.Suffix
	if suffix.IsNone() {
		suffix = pr.TaggedString{Tag: pr.String_, S: ". "}
	}
	suffixS := symbol(suffix)

	value := c.renderValue(counterValue, counter, nil)
	return prefix + value + suffixS
}

type CounterStyleDescriptors struct {
	Negative        [2]pr.TaggedString
	Prefix          pr.TaggedString
	Suffix          pr.TaggedString
	Fallback        string
	System          CounterStyleSystem
	Pad             pr.IntNamedString
	Symbols         []pr.TaggedString
	AdditiveSymbols []pr.IntNamedString
	Range           pr.OptionalRanges
}

func (desc *CounterStyleDescriptors) Validate() error {
	system := desc.System
	if system == (CounterStyleSystem{}) {
		system = CounterStyleSystem{System: "symbolic"}
	}
	if system.Extends == "" {
		switch system.System {
		case "cyclic", "fixed", "symbolic":
			if len(desc.Symbols) == 0 {
				return fmt.Errorf("counter style %s needs at least one symbol", system.System)
			}
		case "alphabetic", "numeric":
			if len(desc.Symbols) < 2 {
				return fmt.Errorf("counter style %s needs at least two symbols", system.System)
			}
		case "additive":
			if len(desc.AdditiveSymbols) < 2 {
				return fmt.Errorf("counter style %s needs at least two additive symbols", system.System)
			}
		}
	}
	return nil
}

func (desc *CounterStyleDescriptors) fallback() string {
	if desc.Fallback != "" {
		return desc.Fallback
	}
	return "decimal"
}

// complete the fields of `desc` with those taken from `src`
func (desc *CounterStyleDescriptors) merge(src CounterStyleDescriptors) {
	if desc.Negative == ([2]pr.TaggedString{}) {
		desc.Negative = src.Negative
	}
	if desc.System == (CounterStyleSystem{}) {
		desc.System = src.System
	}
	if desc.Prefix.IsNone() {
		desc.Prefix = src.Prefix
	}
	if desc.Suffix.IsNone() {
		desc.Suffix = src.Suffix
	}
	if desc.Range.IsNone() {
		desc.Range = src.Range
	}
	if desc.Fallback == "" {
		desc.Fallback = src.Fallback
	}
	if desc.Pad.IsNone() {
		desc.Pad = src.Pad
	}
	if desc.Symbols == nil {
		desc.Symbols = src.Symbols
	}
	if desc.AdditiveSymbols == nil {
		desc.AdditiveSymbols = src.AdditiveSymbols
	}
}

type CounterStyleSystem struct {
	Extends, System string
	Number          int
}

func reverse(a []string) {
	for left, right := 0, len(a)-1; left < right; left, right = left+1, right-1 {
		a[left], a[right] = a[right], a[left]
	}
}
