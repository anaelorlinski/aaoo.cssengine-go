// Package selector is an implementation of CSS selectors.
package selector

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
)

// a parser for CSS selectors
type parser struct {
	s     string // the source text
	i     int    // the current position
	depth int    // current nesting depth of selector lists

	// if `false`, parsing a pseudo-element
	// returns an error.
	acceptPseudoElements bool

	// true while parsing the argument of :has(), which cannot be nested
	inHas bool

	// strict makes an invalid item of a forgiving list an error (see
	// [ParseGroupStrict])
	strict bool
}

// parseEscape parses a backslash escape.
func (p *parser) parseEscape() (result string, err error) {
	if len(p.s) < p.i+2 || p.s[p.i] != '\\' {
		return "", errors.New("invalid escape sequence")
	}

	start := p.i + 1
	c := p.s[start]
	switch {
	case c == '\r' || c == '\n' || c == '\f':
		return "", errors.New("escaped line ending outside string")
	case hexDigit(c):
		// unicode escape (hex)
		var i int
		for i = start; i < start+6 && i < len(p.s) && hexDigit(p.s[i]); i++ {
			// empty
		}
		v, _ := strconv.ParseUint(p.s[start:i], 16, 64)
		if len(p.s) > i {
			switch p.s[i] {
			case '\r':
				i++
				if len(p.s) > i && p.s[i] == '\n' {
					i++
				}
			case ' ', '\t', '\n', '\f':
				i++
			}
		}
		p.i = i
		// CSS escapes for U+0000 denote the replacement character, not a NUL.
		if v == 0 {
			return "\ufffd", nil
		}
		return string(rune(v)), nil
	}

	// Return the literal character after the backslash.
	result = p.s[start : start+1]
	p.i += 2
	return result, nil
}

// toLowerASCII returns s with all ASCII capital letters lowercased.
func toLowerASCII(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if c := s[i]; 'A' <= c && c <= 'Z' {
			if b == nil {
				b = make([]byte, len(s))
				copy(b, s)
			}
			b[i] = s[i] + ('a' - 'A')
		}
	}

	if b == nil {
		return s
	}

	return string(b)
}

func hexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// nameStart returns whether c can be the first character of an identifier
// (not counting an initial hyphen, or an escape sequence).
func nameStart(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_' || c > 127
}

// nameChar returns whether c can be a character within an identifier
// (not counting an escape sequence).
func nameChar(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_' || c > 127 ||
		c == '-' || '0' <= c && c <= '9'
}

// parseIdentifier parses an identifier.
func (p *parser) parseIdentifier() (result string, err error) {
	const prefix = '-'
	var numPrefix int

	for len(p.s) > p.i && p.s[p.i] == prefix {
		p.i++
		numPrefix++
	}

	// two hyphens start an identifier, even with no name after them: "--"
	// (CSS Syntax 3 §4.3.9)
	if numPrefix >= 2 && (len(p.s) <= p.i || !nameChar(p.s[p.i]) && p.s[p.i] != '\\') {
		return strings.Repeat(string(prefix), numPrefix), nil
	}

	if len(p.s) <= p.i {
		return "", errors.New("expected identifier, found EOF instead")
	}

	if c := p.s[p.i]; !(nameStart(c) || c == '\\' || numPrefix >= 2) {
		return "", fmt.Errorf("expected identifier, found %c instead", c)
	}

	result, err = p.parseName()
	if numPrefix > 0 && err == nil {
		result = strings.Repeat(string(prefix), numPrefix) + result
	}
	return
}

// parseName parses a name (which is like an identifier, but doesn't have
// extra restrictions on the first character).
func (p *parser) parseName() (result string, err error) {
	i := p.i
loop:
	for i < len(p.s) {
		c := p.s[i]
		switch {
		case nameChar(c):
			start := i
			for i < len(p.s) && nameChar(p.s[i]) {
				i++
			}
			result += p.s[start:i]
		case c == '\\':
			p.i = i
			val, err := p.parseEscape()
			if err != nil {
				return "", err
			}
			i = p.i
			result += val
		default:
			break loop
		}
	}

	if result == "" {
		return "", errors.New("expected name, found EOF instead")
	}

	p.i = i
	return result, nil
}

// parseString parses a single- or double-quoted string.
func (p *parser) parseString() (result string, err error) {
	i := p.i
	if len(p.s) < i+2 {
		return "", errors.New("expected string, found EOF instead")
	}

	quote := p.s[i]
	i++

loop:
	for i < len(p.s) {
		switch p.s[i] {
		case '\\':
			if len(p.s) > i+1 {
				switch c := p.s[i+1]; c {
				case '\r':
					if len(p.s) > i+2 && p.s[i+2] == '\n' {
						i += 3
						continue loop
					}
					fallthrough
				case '\n', '\f':
					i += 2
					continue loop
				}
			}
			p.i = i
			val, err := p.parseEscape()
			if err != nil {
				return "", err
			}
			i = p.i
			result += val
		case quote:
			break loop
		case '\r', '\n', '\f':
			return "", errors.New("unexpected end of line in string")
		default:
			start := i
			for i < len(p.s) {
				if c := p.s[i]; c == quote || c == '\\' || c == '\r' || c == '\n' || c == '\f' {
					break
				}
				i++
			}
			result += p.s[start:i]
		}
	}

	if i >= len(p.s) {
		return "", errors.New("EOF in string")
	}

	// Consume the final quote.
	i++

	p.i = i
	return result, nil
}

// parseRegex parses a regular expression; the end is defined by encountering an
// unmatched closing ')' or ']' which is not consumed
func (p *parser) parseRegex() (rx *regexp.Regexp, err error) {
	i := p.i
	if len(p.s) < i+2 {
		return nil, errors.New("expected regular expression, found EOF instead")
	}

	// number of open parens or brackets;
	// when it becomes negative, finished parsing regex
	open := 0

loop:
	for i < len(p.s) {
		switch p.s[i] {
		case '(', '[':
			open++
		case ')', ']':
			open--
			if open < 0 {
				break loop
			}
		}
		i++
	}

	if i >= len(p.s) {
		return nil, errors.New("EOF in regular expression")
	}
	rx, err = regexp.Compile(p.s[p.i:i])
	p.i = i
	return rx, err
}

// skipWhitespace consumes whitespace characters and comments.
// It returns true if there was actually anything to skip.
func (p *parser) skipWhitespace() bool {
	i := p.i
	for i < len(p.s) {
		switch p.s[i] {
		case ' ', '\t', '\r', '\n', '\f':
			i++
			continue
		case '/':
			if strings.HasPrefix(p.s[i:], "/*") {
				end := strings.Index(p.s[i+len("/*"):], "*/")
				if end != -1 {
					i += end + len("/**/")
					continue
				}
			}
		}
		break
	}

	if i > p.i {
		p.i = i
		return true
	}

	return false
}

// consumeParenthesis consumes an opening parenthesis and any following
// whitespace. It returns true if there was actually a parenthesis to skip.
func (p *parser) consumeParenthesis() bool {
	if p.i < len(p.s) && p.s[p.i] == '(' {
		p.i++
		p.skipWhitespace()
		return true
	}
	return false
}

// consumeClosingParenthesis consumes a closing parenthesis and any preceding
// whitespace. It returns true if there was actually a parenthesis to skip.
func (p *parser) consumeClosingParenthesis() bool {
	i := p.i
	p.skipWhitespace()
	if p.i < len(p.s) && p.s[p.i] == ')' {
		p.i++
		return true
	}
	p.i = i
	return false
}

// isNamespaceBar reports whether the input at p.i is the '|' of a
// namespace prefix (and not the start of the "|=" operator).
func (p *parser) isNamespaceBar() bool {
	return p.i < len(p.s) && p.s[p.i] == '|' && !(p.i+1 < len(p.s) && p.s[p.i+1] == '=')
}

// resolveNamespacePrefix maps a namespace prefix to a namespace name. See
// knownNamespacePrefixes: @namespace is not supported, so only the fixed
// prefixes are declared.
func resolveNamespacePrefix(prefix string) (string, error) {
	if ns, ok := knownNamespacePrefixes[prefix]; ok {
		return ns, nil
	}
	return "", fmt.Errorf("undeclared namespace prefix %q", prefix)
}

// parseTypeSelector parses a type or universal selector, with an optional
// namespace prefix: "E", "*", "ns|E", "ns|*", "*|E", "*|*", "|E", "|*".
// It returns nil for a universal selector without namespace constraint,
// which does not affect the meaning of the compound.
func (p *parser) parseTypeSelector() (Sel, error) {
	ns := anyNamespace
	var name string // "" for '*'
	switch {
	case p.s[p.i] == '*':
		p.i++
		if !p.isNamespaceBar() {
			return nil, nil
		}
	case p.s[p.i] == '|':
		ns = ""
	default:
		ident, err := p.parseIdentifier()
		if err != nil {
			return nil, err
		}
		if !p.isNamespaceBar() {
			return newTagSelector(ident), nil
		}
		if ns, err = resolveNamespacePrefix(ident); err != nil {
			return nil, err
		}
	}
	p.i++ // the '|'
	if p.i < len(p.s) && p.s[p.i] == '*' {
		p.i++
	} else {
		ident, err := p.parseIdentifier()
		if err != nil {
			return nil, err
		}
		name = ident
	}
	if name == "" && ns == anyNamespace {
		return nil, nil
	}
	t := newTagSelector(name)
	t.ns = ns
	return t, nil
}

// parseIDSelector parses a selector that matches by id attribute.
func (p *parser) parseIDSelector() (idSelector, error) {
	if p.i >= len(p.s) {
		return idSelector{}, fmt.Errorf("expected id selector (#id), found EOF instead")
	}
	if p.s[p.i] != '#' {
		return idSelector{}, fmt.Errorf("expected id selector (#id), found '%c' instead", p.s[p.i])
	}

	p.i++
	// the name must start like an identifier: "#1" is a hash, not an id
	// selector (CSS Selectors 4 §3, the hash token of type "id")
	if p.i < len(p.s) {
		c := p.s[p.i]
		digitAfterHyphen := c == '-' && p.i+1 < len(p.s) && '0' <= p.s[p.i+1] && p.s[p.i+1] <= '9'
		loneHyphen := c == '-' && (p.i+1 == len(p.s) || !nameChar(p.s[p.i+1]) && p.s[p.i+1] != '\\')
		if '0' <= c && c <= '9' || digitAfterHyphen || loneHyphen {
			return idSelector{}, fmt.Errorf("expected id selector, found #%c", c)
		}
	}
	id, err := p.parseName()
	if err != nil {
		return idSelector{}, err
	}

	return idSelector{id: id}, nil
}

// parseClassSelector parses a selector that matches by class attribute.
func (p *parser) parseClassSelector() (classSelector, error) {
	if p.i >= len(p.s) {
		return classSelector{}, fmt.Errorf("expected class selector (.class), found EOF instead")
	}
	if p.s[p.i] != '.' {
		return classSelector{}, fmt.Errorf("expected class selector (.class), found '%c' instead", p.s[p.i])
	}

	p.i++
	class, err := p.parseIdentifier()
	if err != nil {
		return classSelector{}, err
	}

	return classSelector{class: class}, nil
}

// parseAttributeSelector parses a selector that matches by attribute value.
func (p *parser) parseAttributeSelector() (attrSelector, error) {
	if p.i >= len(p.s) {
		return attrSelector{}, fmt.Errorf("expected attribute selector ([attribute]), found EOF instead")
	}
	if p.s[p.i] != '[' {
		return attrSelector{}, fmt.Errorf("expected attribute selector ([attribute]), found '%c' instead", p.s[p.i])
	}

	p.i++
	p.skipWhitespace()
	// attribute name, with an optional namespace prefix: "ns|a", "*|a", "|a"
	ns := ""
	if p.i+1 < len(p.s) && p.s[p.i] == '*' && p.s[p.i+1] == '|' {
		ns = anyNamespace
		p.i += 2
	} else if p.isNamespaceBar() {
		p.i++
	}
	key, err := p.parseIdentifier()
	if err != nil {
		return attrSelector{}, err
	}
	if p.isNamespaceBar() {
		if ns, err = resolveNamespacePrefix(key); err != nil {
			return attrSelector{}, err
		}
		if ns == htmlNamespace {
			ns = "" // HTML attributes are in no namespace
		}
		p.i++
		if key, err = p.parseIdentifier(); err != nil {
			return attrSelector{}, err
		}
	}
	lowerKey := toLowerASCII(key)

	p.skipWhitespace()
	if p.i >= len(p.s) {
		return attrSelector{}, errors.New("unexpected EOF in attribute selector")
	}

	if p.s[p.i] == ']' {
		p.i++
		return attrSelector{key: key, lowerKey: lowerKey, ns: ns, operation: ""}, nil
	}

	if p.i+2 >= len(p.s) {
		return attrSelector{}, errors.New("unexpected EOF in attribute selector")
	}

	op := p.s[p.i : p.i+2]
	if op[0] == '=' {
		op = "="
	} else if op[1] != '=' {
		return attrSelector{}, fmt.Errorf(`expected equality operator, found "%s" instead`, op)
	}
	p.i += len(op)

	p.skipWhitespace()
	if p.i >= len(p.s) {
		return attrSelector{}, errors.New("unexpected EOF in attribute selector")
	}
	var val string
	var rx *regexp.Regexp
	if op == "#=" {
		rx, err = p.parseRegex()
	} else {
		switch p.s[p.i] {
		case '\'', '"':
			val, err = p.parseString()
		default:
			val, err = p.parseIdentifier()
		}
	}
	if err != nil {
		return attrSelector{}, err
	}

	p.skipWhitespace()
	if p.i >= len(p.s) {
		return attrSelector{}, errors.New("unexpected EOF in attribute selector")
	}

	// the case-sensitivity flag: "i" (ASCII case-insensitive) or "s" (case-sensitive)
	ignoreCase, caseSensitive := false, false
	switch p.s[p.i] {
	case 'i', 'I':
		ignoreCase = true
		p.i++
	case 's', 'S':
		caseSensitive = true
		p.i++
	}

	p.skipWhitespace()
	if p.i >= len(p.s) {
		return attrSelector{}, errors.New("unexpected EOF in attribute selector")
	}

	if p.s[p.i] != ']' {
		return attrSelector{}, fmt.Errorf("expected ']', found '%c' instead", p.s[p.i])
	}
	p.i++

	switch op {
	case "=", "!=", "~=", "|=", "^=", "$=", "*=", "#=":
		return attrSelector{
			key: key, lowerKey: lowerKey, ns: ns, val: val, operation: op, regexp: rx,
			ignoreCase: ignoreCase, caseSensitive: caseSensitive,
			htmlIgnoreCase: !caseSensitive && (ns == "" || ns == anyNamespace) && htmlCaseInsensitiveAttributes[lowerKey],
		}, nil
	default:
		return attrSelector{}, fmt.Errorf("attribute operator %q is not supported", op)
	}
}

var (
	errExpectedParenthesis        = errors.New("expected '(' but didn't find it")
	errExpectedClosingParenthesis = errors.New("expected ')' but didn't find it")
	errUnmatchedParenthesis       = errors.New("unmatched '('")
)

// parsePseudoclassSelector parses a pseudoclass selector like :not(p) or a pseudo-element
// For backwards compatibility, both ':' and '::' prefix are allowed for pseudo-elements.
// https://drafts.csswg.org/selectors-3/#pseudo-elements
// Returning a nil `Sel` (and a nil `error`) means we found a pseudo-element.
func (p *parser) parsePseudoclassSelector() (out Sel, pseudoElement string, err error) {
	if p.i >= len(p.s) {
		return nil, "", fmt.Errorf("expected pseudoclass selector (:pseudoclass), found EOF instead")
	}
	if p.s[p.i] != ':' {
		return nil, "", fmt.Errorf("expected attribute selector (:pseudoclass), found '%c' instead", p.s[p.i])
	}

	p.i++
	var mustBePseudoElement bool
	if p.i >= len(p.s) {
		return nil, "", fmt.Errorf("got empty pseudoclass (or pseudoelement)")
	}
	if p.s[p.i] == ':' { // we found a pseudo-element
		mustBePseudoElement = true
		p.i++
	}

	name, err := p.parseIdentifier()
	if err != nil {
		return
	}
	name = toLowerASCII(name)
	if mustBePseudoElement {
		switch name {
		case "after", "backdrop", "before", "cue", "first-letter", "first-line", "grammar-error",
			"marker", "placeholder", "selection", "spelling-error", "footnote-call", "footnote-marker":
			// OK
		default:
			return out, "", fmt.Errorf("unknown pseudo-element :%s", name)
		}
	}

	switch name {
	case "is", "where", "not":
		if !p.consumeParenthesis() {
			return out, "", errExpectedParenthesis
		}
		var (
			sel      SelectorGroup
			parseErr error
		)
		if name == "not" {
			sel, parseErr = p.parseArgumentSelectorGroup()
		} else {
			sel, parseErr = p.parseForgivingSelectorGroup()
		}
		if parseErr != nil {
			return out, "", parseErr
		}
		if !p.consumeClosingParenthesis() {
			return out, "", errExpectedClosingParenthesis
		}

		out = logicalPseudoClassSelector{name: name, match: sel, nested: groupHasNested(sel), cacheable: groupUsesCache(sel)}

	case "has", "haschild":
		// https://drafts.csswg.org/selectors-4/#relational : :has() cannot be nested.
		if p.inHas {
			return out, "", fmt.Errorf(":%s() is not allowed inside :has()", name)
		}
		if !p.consumeParenthesis() {
			return out, "", errExpectedParenthesis
		}
		p.inHas = true
		rels, parseErr := p.parseRelativeSelectorGroup()
		p.inHas = false
		if parseErr != nil {
			return out, "", parseErr
		}
		if !p.consumeClosingParenthesis() {
			return out, "", errExpectedClosingParenthesis
		}

		out = hasPseudoClassSelector{name: name, rels: rels}

	case "contains", "containsown":
		if !p.consumeParenthesis() {
			return out, "", errExpectedParenthesis
		}
		if p.i == len(p.s) {
			return out, "", errUnmatchedParenthesis
		}
		var val string
		switch p.s[p.i] {
		case '\'', '"':
			val, err = p.parseString()
		default:
			val, err = p.parseIdentifier()
		}
		if err != nil {
			return out, "", err
		}
		val = strings.ToLower(val)
		p.skipWhitespace()
		if p.i >= len(p.s) {
			return out, "", errors.New("unexpected EOF in pseudo selector")
		}
		if !p.consumeClosingParenthesis() {
			return out, "", errExpectedClosingParenthesis
		}

		out = containsPseudoClassSelector{own: name == "containsown", value: val}

	case "matches", "matchesown":
		if !p.consumeParenthesis() {
			return out, "", errExpectedParenthesis
		}
		rx, err := p.parseRegex()
		if err != nil {
			return out, "", err
		}
		if p.i >= len(p.s) {
			return out, "", errors.New("unexpected EOF in pseudo selector")
		}
		if !p.consumeClosingParenthesis() {
			return out, "", errExpectedClosingParenthesis
		}

		out = regexpPseudoClassSelector{own: name == "matchesown", regexp: rx}

	case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
		if !p.consumeParenthesis() {
			return out, "", errExpectedParenthesis
		}
		a, b, err := p.parseNth()
		if err != nil {
			return out, "", err
		}
		last := name == "nth-last-child" || name == "nth-last-of-type"
		ofType := name == "nth-of-type" || name == "nth-last-of-type"
		// :nth-child(An+B of S), https://drafts.csswg.org/selectors-4/#the-nth-child-pseudo
		var of SelectorGroup
		if !ofType && p.consumeOf() {
			if of, err = p.parseArgumentSelectorGroup(); err != nil {
				return out, "", err
			}
		}
		if !p.consumeClosingParenthesis() {
			return out, "", errExpectedClosingParenthesis
		}
		out = nthPseudoClassSelector{a: a, b: b, last: last, ofType: ofType, of: of, nested: of != nil}

	case "first-child":
		out = nthPseudoClassSelector{a: 0, b: 1, ofType: false, last: false}
	case "last-child":
		out = nthPseudoClassSelector{a: 0, b: 1, ofType: false, last: true}
	case "first-of-type":
		out = nthPseudoClassSelector{a: 0, b: 1, ofType: true, last: false}
	case "last-of-type":
		out = nthPseudoClassSelector{a: 0, b: 1, ofType: true, last: true}
	case "only-child":
		out = onlyChildPseudoClassSelector{ofType: false}
	case "only-of-type":
		out = onlyChildPseudoClassSelector{ofType: true}
	case "input":
		out = inputPseudoClassSelector{}
	case "empty":
		out = emptyElementPseudoClassSelector{}
	case "root":
		out = rootPseudoClassSelector{}
	case "link":
		out = linkPseudoClassSelector{}
	case "lang":
		if !p.consumeParenthesis() {
			return out, "", errExpectedParenthesis
		}
		// a comma-separated list of language ranges, as identifiers or strings
		var ranges []string
		for {
			if p.i == len(p.s) {
				return out, "", errUnmatchedParenthesis
			}
			var val string
			switch p.s[p.i] {
			case '\'', '"':
				val, err = p.parseString()
			default:
				val, err = p.parseIdentifier()
			}
			if err != nil {
				return out, "", err
			}
			ranges = append(ranges, toLowerASCII(val))
			p.skipWhitespace()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				p.skipWhitespace()
				continue
			}
			break
		}
		if p.i >= len(p.s) {
			return out, "", errors.New("unexpected EOF in pseudo selector")
		}
		if !p.consumeClosingParenthesis() {
			return out, "", errExpectedClosingParenthesis
		}
		out = langPseudoClassSelector{ranges: ranges}
	case "enabled":
		out = enabledPseudoClassSelector{}
	case "disabled":
		out = disabledPseudoClassSelector{}
	case "checked":
		out = checkedPseudoClassSelector{}
	case "dir":
		if !p.consumeParenthesis() {
			return out, "", errExpectedParenthesis
		}
		dir, err := p.parseIdentifier()
		if err != nil {
			return out, "", err
		}
		if !p.consumeClosingParenthesis() {
			return out, "", errExpectedClosingParenthesis
		}
		// values other than ltr and rtl are valid but match nothing
		out = statePseudoClassSelector{name: "dir", dir: toLowerASCII(dir)}
	case "host", "state", "host-context":
		// the shadow DOM, custom states: nothing matches in a document
		// without shadow trees or custom elements
		value := ":" + name
		if p.i < len(p.s) && p.s[p.i] == '(' {
			start := p.i
			p.i++
			if !p.skipListItem() || p.s[p.i] != ')' {
				return out, "", errExpectedClosingParenthesis
			}
			p.i++
			value += p.s[start:p.i]
		} else if name != "host" {
			return out, "", errExpectedParenthesis
		}
		out = neverMatchSelector{value: value}
	case "after", "backdrop", "before", "cue", "first-letter", "first-line",
		"grammar-error", "marker", "placeholder", "selection", "spelling-error", "footnote-call", "footnote-marker":
		return nil, name, nil
	default:
		switch {
		case statePseudoClasses[name]:
			out = statePseudoClassSelector{name: name}
		case staticNeverMatch[name]:
			// interaction or playback state: never matches in a static
			// rendering
			out = neverMatchSelector{value: ":" + name}
		default:
			return out, "", fmt.Errorf("unknown pseudoclass or pseudoelement :%s", name)
		}
	}
	return
}

// parseInteger parses a  decimal integer.
func (p *parser) parseInteger() (int, error) {
	i := p.i
	start := i
	for i < len(p.s) && '0' <= p.s[i] && p.s[i] <= '9' {
		i++
	}
	if i == start {
		return 0, errors.New("expected integer, but didn't find it")
	}
	p.i = i

	// Clamp to 32 bits, as browsers do: huge an+b coefficients stay valid
	// and the matching arithmetic cannot overflow.
	val, err := strconv.ParseInt(p.s[start:i], 10, 64)
	if err != nil || val > limits.NthCoefficient {
		val = limits.NthCoefficient
	}
	return int(val), nil
}

// consumeOf consumes the " of " keyword of :nth-child(An+B of S) (and the
// whitespace around it). It returns false, consuming nothing, if it is not
// there.
func (p *parser) consumeOf() bool {
	i := p.i
	// "2n+1of" is a dimension, not an+b then of: whitespace must separate
	// them (it may have been consumed already)
	spaced := i > 0 && strings.ContainsRune(" \t\r\n\f", rune(p.s[i-1]))
	p.skipWhitespace()
	if p.i == i && !spaced {
		return false
	}
	if p.i+2 < len(p.s) && (p.s[p.i] == 'o' || p.s[p.i] == 'O') && (p.s[p.i+1] == 'f' || p.s[p.i+1] == 'F') {
		switch p.s[p.i+2] {
		case ' ', '\t', '\r', '\n', '\f', '/': // a comment ends the keyword too
			if p.s[p.i+2] == '/' && !strings.HasPrefix(p.s[p.i+2:], "/*") {
				break
			}
			p.i += 2
			p.skipWhitespace()
			return true
		}
	}
	p.i = i
	return false
}

// parseNth parses the argument for :nth-child (normally of the form an+b).
func (p *parser) parseNth() (a, b int, err error) {
	// initial state
	if p.i >= len(p.s) {
		goto eof
	}
	switch p.s[p.i] {
	case '-':
		p.i++
		goto negativeA
	case '+':
		p.i++
		goto positiveA
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		goto positiveA
	case 'n', 'N':
		a = 1
		p.i++
		goto readN
	case 'o', 'O', 'e', 'E':
		id, nameErr := p.parseName()
		if nameErr != nil {
			return 0, 0, nameErr
		}
		id = toLowerASCII(id)
		if id == "odd" {
			return 2, 1, nil
		}
		if id == "even" {
			return 2, 0, nil
		}
		return 0, 0, fmt.Errorf("expected 'odd' or 'even', but found '%s' instead", id)
	default:
		goto invalid
	}

positiveA:
	if p.i >= len(p.s) {
		goto eof
	}
	switch p.s[p.i] {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		a, err = p.parseInteger()
		if err != nil {
			return 0, 0, err
		}
		goto readA
	case 'n', 'N':
		a = 1
		p.i++
		goto readN
	default:
		goto invalid
	}

negativeA:
	if p.i >= len(p.s) {
		goto eof
	}
	switch p.s[p.i] {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		a, err = p.parseInteger()
		if err != nil {
			return 0, 0, err
		}
		a = -a
		goto readA
	case 'n', 'N':
		a = -1
		p.i++
		goto readN
	default:
		goto invalid
	}

readA:
	if p.i >= len(p.s) {
		goto eof
	}
	switch p.s[p.i] {
	case 'n', 'N':
		p.i++
		goto readN
	default:
		// The number we read as a is actually b.
		return 0, a, nil
	}

readN:
	p.skipWhitespace()
	if p.i >= len(p.s) {
		goto eof
	}
	switch p.s[p.i] {
	case '+':
		p.i++
		p.skipWhitespace()
		b, err = p.parseInteger()
		if err != nil {
			return 0, 0, err
		}
		return a, b, nil
	case '-':
		p.i++
		p.skipWhitespace()
		b, err = p.parseInteger()
		if err != nil {
			return 0, 0, err
		}
		return a, -b, nil
	default:
		return a, 0, nil
	}

eof:
	return 0, 0, errors.New("unexpected EOF while attempting to parse expression of form an+b")

invalid:
	return 0, 0, errors.New("unexpected character while attempting to parse expression of form an+b")
}

// parseSimpleSelectorSequence parses a selector sequence that applies to
// a single element.
func (p *parser) parseSimpleSelectorSequence() (Sel, error) {
	var selectors []Sel

	if p.i >= len(p.s) {
		return nil, errors.New("expected selector, found EOF instead")
	}

	switch p.s[p.i] {
	case '#', '.', '[', ':':
		// There's no type selector. Wait to process the other till the main loop.
	default:
		// A type selector, or the universal selector (which is skipped,
		// unless it carries a namespace constraint).
		r, err := p.parseTypeSelector()
		if err != nil {
			return nil, err
		}
		if r != nil {
			selectors = append(selectors, r)
		}
	}

	var pseudoElement string
loop:
	for p.i < len(p.s) {
		var (
			ns               Sel
			newPseudoElement string
			err              error
		)
		switch p.s[p.i] {
		case '#':
			ns, err = p.parseIDSelector()
		case '.':
			ns, err = p.parseClassSelector()
		case '[':
			ns, err = p.parseAttributeSelector()
		case ':':
			ns, newPseudoElement, err = p.parsePseudoclassSelector()
		default:
			break loop
		}
		if err != nil {
			return nil, err
		}
		// From https://drafts.csswg.org/selectors-3/#pseudo-elements :
		// "Only one pseudo-element may appear per selector, and if present
		// it must appear after the sequence of simple selectors that
		// represents the subjects of the selector.""
		if ns == nil { // we found a pseudo-element
			if pseudoElement != "" {
				return nil, fmt.Errorf("only one pseudo-element is accepted per selector, got %s and %s", pseudoElement, newPseudoElement)
			}
			if !p.acceptPseudoElements {
				return nil, fmt.Errorf("pseudo-element %s found, but pseudo-elements support is disabled", newPseudoElement)
			}
			pseudoElement = newPseudoElement
		} else {
			if pseudoElement != "" {
				return nil, fmt.Errorf("pseudo-element %s must be at the end of selector", pseudoElement)
			}
			selectors = append(selectors, ns)
		}

	}
	if len(selectors) == 1 && pseudoElement == "" { // no need wrap the selectors in compoundSelector
		return selectors[0], nil
	}
	return newCompoundSelector(selectors, pseudoElement), nil
}

// parseSelector parses a selector that may include combinators.
func (p *parser) parseSelector() (Sel, error) {
	p.skipWhitespace()
	first, err := p.parseSimpleSelectorSequence()
	if err != nil {
		return nil, err
	}
	compounds := []Sel{first}
	var combinators []byte
	result := func() Sel {
		if len(compounds) == 1 {
			return compounds[0]
		}
		return newComplexSelector(compounds, combinators)
	}

	for {
		var (
			combinator byte
			c          Sel
		)
		if p.skipWhitespace() {
			combinator = ' '
		}
		if p.i >= len(p.s) {
			return result(), nil
		}

		switch p.s[p.i] {
		case '+', '>', '~':
			combinator = p.s[p.i]
			p.i++
			p.skipWhitespace()
		case ',', ')':
			// These characters can't begin a selector, but they can legally occur after one.
			return result(), nil
		}

		if combinator == 0 {
			return result(), nil
		}

		// A pseudo-element must end the selector: "div::before p" is invalid.
		if pe := compounds[len(compounds)-1].PseudoElement(); pe != "" {
			return nil, fmt.Errorf("pseudo-element %s must be at the end of selector", pe)
		}

		c, err = p.parseSimpleSelectorSequence()
		if err != nil {
			return nil, err
		}
		compounds = append(compounds, c)
		combinators = append(combinators, combinator)
	}
}

// parseSelectorGroup parses a group of selectors, separated by commas.
func (p *parser) parseSelectorGroup() (SelectorGroup, error) {
	if p.depth > limits.NestingDepth {
		return nil, errTooDeep
	}
	p.depth++
	defer func() { p.depth-- }()

	current, err := p.parseSelector()
	if err != nil {
		return nil, err
	}
	result := SelectorGroup{current}

	for p.i < len(p.s) {
		if p.s[p.i] != ',' {
			break
		}
		p.i++
		c, err := p.parseSelector()
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}

// parseArgumentSelectorGroup parses the selector list argument of a
// functional pseudo-class (:is, :where, :not). Pseudo-elements are not valid
// there, whatever the top-level setting.
func (p *parser) parseArgumentSelectorGroup() (SelectorGroup, error) {
	accept := p.acceptPseudoElements
	p.acceptPseudoElements = false
	defer func() { p.acceptPseudoElements = accept }()
	return p.parseSelectorGroup()
}

// errTooDeep reports a selector nested deeper than limits.NestingDepth. Unlike
// the other errors, a forgiving selector list does not drop it: the limit
// bounds the work and the stack, whatever the selector.
var errTooDeep = fmt.Errorf("selector nesting too deep (max %d)", limits.NestingDepth)

// parseForgivingSelectorGroup parses the <forgiving-selector-list> argument
// of :is() and :where() (https://drafts.csswg.org/selectors-4/#typedef-forgiving-selector-list):
// the invalid items are dropped, and the list may be empty (it then matches
// nothing). It stops before the closing parenthesis.
func (p *parser) parseForgivingSelectorGroup() (SelectorGroup, error) {
	accept := p.acceptPseudoElements
	p.acceptPseudoElements = false
	defer func() { p.acceptPseudoElements = accept }()

	if p.depth > limits.NestingDepth {
		return nil, errTooDeep
	}
	p.depth++
	defer func() { p.depth-- }()

	result := SelectorGroup{}
	for {
		start := p.i
		sel, err := p.parseSelector()
		if errors.Is(err, errTooDeep) {
			return nil, err
		}
		p.skipWhitespace()
		if err == nil && p.i < len(p.s) && (p.s[p.i] == ',' || p.s[p.i] == ')') {
			result = append(result, sel)
		} else {
			if p.strict {
				if err == nil {
					err = fmt.Errorf("invalid item in a forgiving list at %d", start)
				}
				return nil, err
			}
			p.i = start
			if !p.skipListItem() {
				return nil, errExpectedClosingParenthesis
			}
		}
		if p.i < len(p.s) && p.s[p.i] == ',' {
			p.i++
			continue
		}
		return result, nil
	}
}

// skipListItem advances to the ',' or ')' ending the current item of a
// selector list, skipping nested blocks and strings. It reports false at
// the end of the input.
func (p *parser) skipListItem() bool {
	var open []byte // the blocks entered, innermost last
	for p.i < len(p.s) {
		switch c := p.s[p.i]; c {
		case '\\':
			p.i++ // the escaped character
		case '"', '\'':
			for p.i++; p.i < len(p.s) && p.s[p.i] != c; p.i++ {
				if p.s[p.i] == '\\' {
					p.i++
				}
			}
		case '/':
			if p.i+1 < len(p.s) && p.s[p.i+1] == '*' { // a comment
				end := strings.Index(p.s[p.i+2:], "*/")
				if end < 0 {
					return false
				}
				p.i += 2 + end + 1
			}
		case '(', '[':
			open = append(open, c)
		case ']':
			// a stray ] closes nothing
			if len(open) > 0 && open[len(open)-1] == '[' {
				open = open[:len(open)-1]
			}
		case ')':
			if len(open) == 0 {
				return true
			}
			if open[len(open)-1] == '(' {
				open = open[:len(open)-1]
			}
		case ',':
			if len(open) == 0 {
				return true
			}
		}
		p.i++
	}
	return false
}

// parseRelativeSelector parses a selector that may start with a combinator
// (a relative selector, as used inside :has()).
func (p *parser) parseRelativeSelector() (relativeSelector, error) {
	p.skipWhitespace()
	var combinator byte
	if p.i < len(p.s) {
		switch p.s[p.i] {
		case '+', '>', '~':
			combinator = p.s[p.i]
			p.i++
			p.skipWhitespace()
		}
	}
	sel, err := p.parseSelector()
	if err != nil {
		return relativeSelector{}, err
	}
	return relativeSelector{combinator: combinator, sel: sel, chain: asComplex(sel)}, nil
}

// parseRelativeSelectorGroup parses a comma-separated list of relative
// selectors. Pseudo-elements are not valid inside :has().
func (p *parser) parseRelativeSelectorGroup() ([]relativeSelector, error) {
	if p.depth > limits.NestingDepth {
		return nil, errTooDeep
	}
	p.depth++
	defer func() { p.depth-- }()

	accept := p.acceptPseudoElements
	p.acceptPseudoElements = false
	defer func() { p.acceptPseudoElements = accept }()

	current, err := p.parseRelativeSelector()
	if err != nil {
		return nil, err
	}
	result := []relativeSelector{current}

	for p.i < len(p.s) {
		if p.s[p.i] != ',' {
			break
		}
		p.i++
		c, err := p.parseRelativeSelector()
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}
