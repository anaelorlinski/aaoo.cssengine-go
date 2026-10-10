package selector

import (
	"fmt"
	"strconv"
	"strings"
)

// implements the reverse operation Sel -> string

// escape serializes an identifier, as CSSOM does
// (https://drafts.csswg.org/cssom/#serialize-an-identifier): the output
// parses back to s.
func escape(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == 0:
			b.WriteRune('\uFFFD')
		case r < 0x20 || r == 0x7F,
			i == 0 && '0' <= r && r <= '9',
			i == 1 && '0' <= r && r <= '9' && s[0] == '-':
			fmt.Fprintf(&b, "\\%x ", r)
		case i == 0 && r == '-' && len(s) == 1:
			b.WriteString("\\-")
		case r >= 0x80, r == '-', r == '_', '0' <= r && r <= '9', 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z':
			b.WriteRune(r)
		default:
			b.WriteByte('\\')
			b.WriteRune(r)
		}
	}
	return b.String()
}

// quote serializes a string, as CSSOM does
// (https://drafts.csswg.org/cssom/#serialize-a-string).
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == 0:
			b.WriteRune('\uFFFD')
		case r < 0x20 || r == 0x7F:
			fmt.Fprintf(&b, "\\%x ", r)
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// namespacePrefix serializes a namespace constraint as a prefix, given the
// default constraint of the selector kind.
func namespacePrefix(ns, def string) string {
	switch ns {
	case def:
		return ""
	case anyNamespace:
		return "*|"
	default:
		return ns + "|"
	}
}

func (c tagSelector) String() string {
	name := escape(c.tagS)
	if c.tagS == "" {
		name = "*"
	}
	return namespacePrefix(c.ns, anyNamespace) + name
}

func (c idSelector) String() string {
	return "#" + escape(c.id)
}

func (c classSelector) String() string {
	return "." + escape(c.class)
}

func (c attrSelector) String() string {
	val := c.val
	if c.operation == "#=" {
		val = c.regexp.String()
	} else if c.operation != "" {
		val = quote(val)
	}

	flag := ""
	if c.ignoreCase {
		flag = " i"
	} else if c.caseSensitive {
		flag = " s"
	}

	return fmt.Sprintf(`[%s%s%s%s%s]`, namespacePrefix(c.ns, ""), escape(c.key), c.operation, val, flag)
}

func (c logicalPseudoClassSelector) String() string {
	return fmt.Sprintf(":%s(%s)", c.name, c.match.String())
}

func (c hasPseudoClassSelector) String() string {
	parts := make([]string, len(c.rels))
	for i, r := range c.rels {
		if r.combinator != 0 {
			parts[i] = fmt.Sprintf("%s %s", string(r.combinator), r.sel.String())
		} else {
			parts[i] = r.sel.String()
		}
	}
	return fmt.Sprintf(":%s(%s)", c.name, strings.Join(parts, ", "))
}

func (c containsPseudoClassSelector) String() string {
	s := "contains"
	if c.own {
		s += "Own"
	}
	return fmt.Sprintf(`:%s(%s)`, s, quote(c.value))
}

func (c regexpPseudoClassSelector) String() string {
	s := "matches"
	if c.own {
		s += "Own"
	}
	return fmt.Sprintf(":%s(%s)", s, c.regexp.String())
}

func (c nthPseudoClassSelector) String() string {
	if c.a == 0 && c.b == 1 && c.of == nil { // special cases
		s := ":first-"
		if c.last {
			s = ":last-"
		}
		if c.ofType {
			s += "of-type"
		} else {
			s += "child"
		}
		return s
	}
	var name string
	switch [2]bool{c.last, c.ofType} {
	case [2]bool{true, true}:
		name = "nth-last-of-type"
	case [2]bool{true, false}:
		name = "nth-last-child"
	case [2]bool{false, true}:
		name = "nth-of-type"
	case [2]bool{false, false}:
		name = "nth-child"
	}
	s := fmt.Sprintf("+%d", c.b)
	if c.b < 0 { // avoid +-8 invalid syntax
		s = strconv.Itoa(c.b)
	}
	of := ""
	if c.of != nil {
		of = " of " + c.of.String()
	}
	return fmt.Sprintf(":%s(%dn%s%s)", name, c.a, s, of)
}

func (c onlyChildPseudoClassSelector) String() string {
	if c.ofType {
		return ":only-of-type"
	}
	return ":only-child"
}

func (c inputPseudoClassSelector) String() string {
	return ":input"
}

func (c emptyElementPseudoClassSelector) String() string {
	return ":empty"
}

func (c rootPseudoClassSelector) String() string {
	return ":root"
}

func (c linkPseudoClassSelector) String() string {
	return ":link"
}

func (c langPseudoClassSelector) String() string {
	ranges := make([]string, len(c.ranges))
	for i, r := range c.ranges {
		ranges[i] = quote(r)
	}
	return fmt.Sprintf(":lang(%s)", strings.Join(ranges, ", "))
}

func (c neverMatchSelector) String() string {
	return c.value
}

func (c enabledPseudoClassSelector) String() string {
	return ":enabled"
}

func (c disabledPseudoClassSelector) String() string {
	return ":disabled"
}

func (c checkedPseudoClassSelector) String() string {
	return ":checked"
}

func (c compoundSelector) String() string {
	if len(c.selectors) == 0 && c.pseudoElement == "" {
		return "*"
	}
	chunks := make([]string, len(c.selectors))
	for i, sel := range c.selectors {
		chunks[i] = sel.String()
	}
	s := strings.Join(chunks, "")
	if c.pseudoElement != "" {
		s += "::" + c.pseudoElement
	}
	return s
}

func (c complexSelector) String() string {
	var b strings.Builder
	for i, compound := range c.compounds {
		if i > 0 {
			if comb := c.combinators[i-1]; comb == ' ' {
				b.WriteByte(' ')
			} else {
				b.WriteString(" " + string(comb) + " ")
			}
		}
		b.WriteString(compound.String())
	}
	return b.String()
}

func (c SelectorGroup) String() string {
	ck := make([]string, len(c))
	for i, s := range c {
		ck[i] = s.String()
	}
	return strings.Join(ck, ", ")
}
