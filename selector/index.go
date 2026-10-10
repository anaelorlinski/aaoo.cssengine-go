package selector

import (
	"slices"

	"golang.org/x/net/html"
)

// KeyKind is the kind of an [IndexKey].
type KeyKind uint8

const (
	// KeyNone: the subject has no id, class or type selector.
	KeyNone KeyKind = iota
	// KeyTag: Value is the ASCII-lowercased type selector.
	KeyTag
	// KeyClass: Value is a class the subject requires.
	KeyClass
	// KeyID: Value is the id the subject requires.
	KeyID
)

// IndexKey is a condition that every element matching a selector meets,
// used to index rules: matching only needs to test the selectors whose key
// the element has. It is the most selective of the id, a class, or the
// type selector of the subject (the rightmost compound).
type IndexKey struct {
	Kind  KeyKind
	Value string
}

// Key returns the index key of s.
func Key(s Sel) IndexKey {
	subject := s
	if c, ok := s.(complexSelector); ok {
		subject = c.compounds[len(c.compounds)-1]
	}
	simples := []Sel{subject}
	if c, ok := subject.(compoundSelector); ok {
		simples = c.selectors
	}
	var key IndexKey
	for _, simple := range simples {
		var candidate IndexKey
		switch simple := simple.(type) {
		case idSelector:
			candidate = IndexKey{KeyID, simple.id}
		case classSelector:
			candidate = IndexKey{KeyClass, simple.class}
		case tagSelector:
			if simple.tagS != "" { // not ns|*
				candidate = IndexKey{KeyTag, simple.lower}
			}
		}
		if candidate.Kind > key.Kind {
			key = candidate
		}
	}
	return key
}

// ElementKeys returns the index keys element has: [KeyNone] (universal
// selectors), its type, its id and each of its classes, once each. They
// are normalized like the id, class and type selectors match: a selector
// can only match an element having its [Key].
func ElementKeys(element *html.Node) []IndexKey {
	// the type as tagSelector.Match compares it: by atom on HTML elements
	tag := toLowerASCII(element.Data)
	if element.DataAtom != 0 && isHTMLElement(element) {
		tag = element.DataAtom.String()
	}
	keys := []IndexKey{{}, {Kind: KeyTag, Value: tag}}
	for _, attr := range element.Attr {
		if attr.Namespace != "" {
			continue
		}
		switch attr.Key {
		case "id":
			if key := (IndexKey{Kind: KeyID, Value: attr.Val}); attr.Val != "" && !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		case "class":
			// split like matchInclude
			for s := attr.Val; s != ""; {
				class := s
				if i := spaceAsciiSet.index(s); i != -1 {
					class, s = s[:i], s[i+1:]
				} else {
					s = ""
				}
				if key := (IndexKey{Kind: KeyClass, Value: class}); class != "" && !slices.Contains(keys, key) {
					keys = append(keys, key)
				}
			}
		}
	}
	return keys
}
