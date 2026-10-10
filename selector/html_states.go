package selector

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The pseudo-classes of HTML element states (HTML §4.16.3, Selectors 4 §9
// to §14), evaluated on the static document: the attributes as written,
// no user interaction. Validity (:valid, :invalid) only knows the
// constraints visible in the markup: a required control without a value is
// invalid; type, pattern and length constraints are not checked.

// statePseudoClassSelector is one of the state pseudo-classes, by name.
type statePseudoClassSelector struct {
	abstractPseudoClass
	name string
	dir  string // the argument of :dir()
}

// statePseudoClasses are the names handled by [statePseudoClassSelector]
// (:dir() aside).
var statePseudoClasses = map[string]bool{
	"any-link": true, "scope": true, "defined": true, "open": true,
	"required": true, "optional": true, "read-only": true, "read-write": true,
	"placeholder-shown": true, "default": true, "indeterminate": true,
	"valid": true, "invalid": true,
}

// staticNeverMatch are the pseudo-classes of user interaction, playback or
// navigation state, which match nothing in a static rendering.
var staticNeverMatch = map[string]bool{
	"visited": true, "hover": true, "active": true, "focus": true, "target": true,
	"focus-visible": true, "focus-within": true, "target-within": true,
	"local-link": true, "current": true, "past": true, "future": true,
	"playing": true, "paused": true, "seeking": true, "buffering": true, "stalled": true,
	"muted": true, "volume-locked": true, "user-valid": true, "user-invalid": true,
	"autofill": true, "modal": true, "fullscreen": true, "picture-in-picture": true,
	"popover-open": true, "closed": true, "blank": true,
}

// usesDocumentCache reports whether the pseudo-class looks beyond the
// element (radio groups, form owners): a [Cache] then memoizes it.
func (s statePseudoClassSelector) usesDocumentCache() bool {
	switch s.name {
	case "default", "indeterminate", "valid", "invalid":
		return true
	}
	return false
}

func (s statePseudoClassSelector) String() string {
	if s.name == "dir" {
		return ":dir(" + s.dir + ")"
	}
	return ":" + s.name
}

func (s statePseudoClassSelector) Match(n *html.Node) bool { return s.match(n, nil) }

func (s statePseudoClassSelector) matchIn(n *html.Node, mctx *matchContext) bool {
	return s.match(n, mctx.cache)
}

// match matches n, cache (possibly nil) memoizing the document scans.
func (s statePseudoClassSelector) match(n *html.Node, cache *Cache) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch s.name {
	case "any-link":
		return linkPseudoClassSelector{}.Match(n)
	case "scope":
		// outside @scope, the scoping root is the root element
		return rootPseudoClassSelector{}.Match(n)
	case "defined":
		// built-in elements; custom elements (with a hyphen) are not
		// defined without scripts
		return n.Namespace != "" || !strings.Contains(n.Data, "-")
	case "open":
		return (n.DataAtom == atom.Details || n.DataAtom == atom.Dialog) && hasAttr(n, "open")
	case "required":
		return requirable(n) && hasAttr(n, "required")
	case "optional":
		return requirable(n) && !hasAttr(n, "required")
	case "read-write":
		return readWrite(n)
	case "read-only":
		return !readWrite(n)
	case "placeholder-shown":
		return placeholderShown(n)
	case "default":
		return isDefault(n, cache)
	case "indeterminate":
		return indeterminate(n, cache)
	case "valid", "invalid":
		if !validatable(n) {
			return false
		}
		return invalid(n, cache) == (s.name == "invalid")
	case "dir":
		return directionality(n) == s.dir
	}
	return false
}

// inputTypes are the states of the type attribute of input; another
// value is the text state.
var inputTypes = map[string]bool{
	"hidden": true, "text": true, "search": true, "tel": true, "url": true, "email": true,
	"password": true, "date": true, "month": true, "week": true, "time": true,
	"datetime-local": true, "number": true, "range": true, "color": true, "checkbox": true,
	"radio": true, "file": true, "submit": true, "image": true, "reset": true, "button": true,
}

// inputType returns the state of the type attribute of an input, "" for
// another element.
func inputType(n *html.Node) string {
	if n.DataAtom != atom.Input {
		return ""
	}
	if v, ok := attr(n, "type"); ok {
		if t := toLowerASCII(strings.TrimSpace(v)); inputTypes[t] {
			return t
		}
	}
	return "text"
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// requirable reports whether the required attribute applies to n.
func requirable(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Select, atom.Textarea:
		return true
	case atom.Input:
		switch inputType(n) {
		case "hidden", "range", "color", "submit", "image", "reset", "button":
			return false
		}
		return true
	}
	return false
}

// readonlyApplies reports whether n is an input to which the readonly
// attribute applies: one whose value is text the user edits.
func readonlyApplies(n *html.Node) bool {
	switch inputType(n) {
	case "text", "search", "url", "tel", "email", "password", "date", "month",
		"week", "time", "datetime-local", "number":
		return true
	}
	return false
}

// readWrite implements :read-write: mutable text controls and editing
// hosts.
func readWrite(n *html.Node) bool {
	switch {
	case n.DataAtom == atom.Textarea || readonlyApplies(n):
		return !hasAttr(n, "readonly") && !actuallyDisabled(n)
	case n.DataAtom == atom.Input: // the other controls are never editable
		return false
	}
	for e := n; e != nil && e.Type == html.ElementNode; e = e.Parent {
		if v, ok := attr(e, "contenteditable"); ok {
			v = toLowerASCII(v)
			return v == "" || v == "true" || v == "plaintext-only"
		}
	}
	return false
}

// controlValue returns the value of an input or textarea.
func controlValue(n *html.Node) string {
	if n.DataAtom == atom.Textarea {
		var b strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
			}
		}
		return b.String()
	}
	v, _ := attr(n, "value")
	return v
}

func placeholderShown(n *html.Node) bool {
	switch inputType(n) {
	case "text", "search", "url", "tel", "email", "password", "number":
	default:
		if n.DataAtom != atom.Textarea {
			return false
		}
	}
	if _, ok := attr(n, "placeholder"); !ok {
		return false
	}
	return controlValue(n) == ""
}

// formOwner returns the form n is associated with: the form whose id is
// its form attribute, or else its nearest form ancestor (HTML §4.10.17.3).
func formOwner(n *html.Node, cache *Cache) *html.Node {
	if id, ok := attr(n, "form"); ok {
		if f := elementByID(n, id, cache); f != nil && f.DataAtom == atom.Form {
			return f
		}
		return nil
	}
	for e := n.Parent; e != nil; e = e.Parent {
		if e.Type == html.ElementNode && e.DataAtom == atom.Form {
			return e
		}
	}
	return nil
}

func root(n *html.Node) *html.Node {
	for n.Parent != nil {
		n = n.Parent
	}
	return n
}

// elementByID returns the first element of the tree of n whose id is id.
func elementByID(n *html.Node, id string, cache *Cache) *html.Node {
	r := root(n)
	if cache == nil {
		return firstDescendant(r, func(e *html.Node) bool { v, _ := attr(e, "id"); return v == id })
	}
	if cache.ids == nil || cache.idsRoot != r {
		cache.ids, cache.idsRoot = map[string]*html.Node{}, r
		walkElements(r, func(e *html.Node) {
			if v, ok := attr(e, "id"); ok {
				if _, seen := cache.ids[v]; !seen {
					cache.ids[v] = e
				}
			}
		})
	}
	return cache.ids[id]
}

func walkElements(n *html.Node, f func(*html.Node)) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			f(c)
			walkElements(c, f)
		}
	}
}

func firstDescendant(n *html.Node, f func(*html.Node) bool) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if f(c) {
			return c
		}
		if d := firstDescendant(c, f); d != nil {
			return d
		}
	}
	return nil
}

func isSubmitButton(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Button:
		// a missing or invalid type is the submit state
		t, _ := attr(n, "type")
		t = toLowerASCII(strings.TrimSpace(t))
		return t != "reset" && t != "button"
	case atom.Input:
		t := inputType(n)
		return t == "submit" || t == "image"
	}
	return false
}

// defaultButton returns the default button of form: its first submit
// button in tree order.
func defaultButton(form *html.Node, cache *Cache) *html.Node {
	if cache == nil {
		return firstDescendant(root(form), func(e *html.Node) bool {
			return isSubmitButton(e) && formOwner(e, nil) == form
		})
	}
	if cache.defaultButtons == nil {
		cache.defaultButtons = map[*html.Node]*html.Node{}
		walkElements(root(form), func(e *html.Node) {
			if isSubmitButton(e) {
				if f := formOwner(e, cache); f != nil {
					if _, seen := cache.defaultButtons[f]; !seen {
						cache.defaultButtons[f] = e
					}
				}
			}
		})
	}
	return cache.defaultButtons[form]
}

// isDefault implements :default: checked checkboxes and radios, selected
// options, and the default button of a form.
func isDefault(n *html.Node, cache *Cache) bool {
	switch {
	case n.DataAtom == atom.Option:
		return hasAttr(n, "selected")
	case inputType(n) == "checkbox" || inputType(n) == "radio":
		return hasAttr(n, "checked")
	case isSubmitButton(n):
		form := formOwner(n, cache)
		return form != nil && defaultButton(form, cache) == n
	}
	return false
}

// radioGroupKey identifies a radio button group: same form owner (or, for
// none, same tree) and same name (HTML §4.10.5.1.18).
type radioGroupKey struct {
	owner *html.Node
	name  string
}

// radioGroup is what the constraints need of a group.
type radioGroup struct{ checked, required bool }

func radioKey(n *html.Node, cache *Cache) radioGroupKey {
	name, _ := attr(n, "name")
	owner := formOwner(n, cache)
	if owner == nil {
		owner = root(n)
	}
	return radioGroupKey{owner: owner, name: name}
}

// groupOf returns the group of the radio button n (with a name).
func groupOf(n *html.Node, cache *Cache) radioGroup {
	key := radioKey(n, cache)
	add := func(g *radioGroup, e *html.Node) {
		g.checked = g.checked || hasAttr(e, "checked")
		g.required = g.required || hasAttr(e, "required")
	}
	if cache == nil {
		var g radioGroup
		walkElements(root(n), func(e *html.Node) {
			if inputType(e) == "radio" && radioKey(e, nil) == key {
				add(&g, e)
			}
		})
		return g
	}
	if cache.radioGroups == nil {
		cache.radioGroups = map[radioGroupKey]radioGroup{}
		walkElements(root(n), func(e *html.Node) {
			if inputType(e) == "radio" {
				if name, _ := attr(e, "name"); name != "" {
					k := radioKey(e, cache)
					g := cache.radioGroups[k]
					add(&g, e)
					cache.radioGroups[k] = g
				}
			}
		})
	}
	return cache.radioGroups[key]
}

// radioState returns the group of a radio button, alone without a name.
func radioState(n *html.Node, cache *Cache) radioGroup {
	if name, _ := attr(n, "name"); name == "" {
		return radioGroup{checked: hasAttr(n, "checked"), required: hasAttr(n, "required")}
	}
	return groupOf(n, cache)
}

// indeterminate implements :indeterminate: a progress without value, and a
// radio button whose group has no checked button. (The indeterminate state
// of checkboxes is only set by scripts.)
func indeterminate(n *html.Node, cache *Cache) bool {
	switch {
	case n.DataAtom == atom.Progress:
		return !hasAttr(n, "value")
	case inputType(n) == "radio":
		return !radioState(n, cache).checked
	}
	return false
}

// validatable reports whether :valid and :invalid apply to n: forms,
// fieldsets, and the candidates for constraint validation (HTML
// §4.10.21.1): the submittable elements not barred (disabled, readonly,
// in a datalist, buttons and inputs without a value to submit).
func validatable(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Form, atom.Fieldset:
		return true
	case atom.Select, atom.Textarea, atom.Button, atom.Input:
	default:
		return false
	}
	switch {
	case n.DataAtom == atom.Button && !isSubmitButton(n),
		inputType(n) == "hidden" || inputType(n) == "reset" || inputType(n) == "button",
		n.DataAtom == atom.Textarea && hasAttr(n, "readonly"),
		readonlyApplies(n) && hasAttr(n, "readonly"),
		actuallyDisabled(n):
		return false
	}
	for e := n.Parent; e != nil; e = e.Parent {
		if e.DataAtom == atom.Datalist {
			return false
		}
	}
	return true
}

// invalid reports whether n does not satisfy its constraints: a required
// control without value; a form with an invalid control, a fieldset with
// an invalid descendant.
func invalid(n *html.Node, cache *Cache) bool {
	switch n.DataAtom {
	case atom.Fieldset:
		return firstDescendant(n, func(e *html.Node) bool {
			return e.DataAtom != atom.Form && e.DataAtom != atom.Fieldset && validatable(e) && invalid(e, cache)
		}) != nil
	case atom.Form:
		return firstDescendant(root(n), func(e *html.Node) bool {
			return e.DataAtom != atom.Form && e.DataAtom != atom.Fieldset && validatable(e) &&
				formOwner(e, cache) == n && invalid(e, cache)
		}) != nil
	case atom.Select:
		return hasAttr(n, "required") && selectMissing(n)
	}
	if !requirable(n) {
		return false
	}
	switch inputType(n) {
	case "checkbox":
		return hasAttr(n, "required") && !hasAttr(n, "checked")
	case "radio":
		g := radioState(n, cache)
		return g.required && !g.checked
	}
	return hasAttr(n, "required") && controlValue(n) == ""
}

// options returns the list of options of a select: its option children
// and those of its optgroup children.
func options(sel *html.Node) []*html.Node {
	var out []*html.Node
	for c := sel.FirstChild; c != nil; c = c.NextSibling {
		switch c.DataAtom {
		case atom.Option:
			out = append(out, c)
		case atom.Optgroup:
			for o := c.FirstChild; o != nil; o = o.NextSibling {
				if o.DataAtom == atom.Option {
					out = append(out, o)
				}
			}
		}
	}
	return out
}

// displaySize returns the display size of a select (its size attribute,
// 4 for a multiple select, 1 otherwise).
func displaySize(sel *html.Node) int {
	if v, ok := attr(sel, "size"); ok {
		if size, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && size > 0 {
			return size
		}
	}
	if hasAttr(sel, "multiple") {
		return 4
	}
	return 1
}

func optionDisabled(o *html.Node) bool {
	return hasAttr(o, "disabled") || o.Parent != nil && o.Parent.DataAtom == atom.Optgroup && hasAttr(o.Parent, "disabled")
}

// selectedOptions returns the options of sel whose selectedness is true
// (HTML §4.10.7, the selectedness setting algorithm): the selected ones,
// the last of them for a single select; without any, the first enabled
// option of a single select displayed as a drop-down box.
func selectedOptions(sel *html.Node) []*html.Node {
	opts := options(sel)
	var selected []*html.Node
	for _, o := range opts {
		if hasAttr(o, "selected") {
			selected = append(selected, o)
		}
	}
	if hasAttr(sel, "multiple") {
		return selected
	}
	if len(selected) > 0 {
		return selected[len(selected)-1:]
	}
	if displaySize(sel) == 1 {
		for _, o := range opts {
			if !optionDisabled(o) {
				return []*html.Node{o}
			}
		}
	}
	return nil
}

// optionValue returns the value of an option: its value attribute, or its
// text with whitespace stripped and collapsed.
func optionValue(o *html.Node) string {
	if v, ok := attr(o, "value"); ok {
		return v
	}
	var b strings.Builder
	var text func(n *html.Node)
	text = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
			} else if c.Type == html.ElementNode && c.DataAtom != atom.Script {
				text(c)
			}
		}
	}
	text(o)
	return strings.Join(strings.Fields(b.String()), " ")
}

// selectMissing reports whether a required select is suffering from being
// missing: no option selected, or only its placeholder label option.
func selectMissing(sel *html.Node) bool {
	selected := selectedOptions(sel)
	if len(selected) == 0 {
		return true
	}
	if len(selected) == 1 && !hasAttr(sel, "multiple") && displaySize(sel) == 1 {
		// the placeholder label option: the first option, a child of the
		// select, with an empty value
		opts := options(sel)
		first := opts[0]
		return selected[0] == first && first.Parent == sel && optionValue(first) == ""
	}
	return false
}

// optionSelected reports whether the selectedness of the option o is true.
func optionSelected(o *html.Node) bool {
	sel := o.Parent
	if sel != nil && sel.DataAtom == atom.Optgroup {
		sel = sel.Parent
	}
	if sel == nil || sel.DataAtom != atom.Select {
		return hasAttr(o, "selected")
	}
	for _, s := range selectedOptions(sel) {
		if s == o {
			return true
		}
	}
	return false
}

// directionality returns "ltr" or "rtl", from the dir attribute of n or its
// nearest ancestor having one (dir="auto" is taken as "ltr": the text is
// not analysed).
func directionality(n *html.Node) string {
	for e := n; e != nil && e.Type == html.ElementNode; e = e.Parent {
		if v, ok := attr(e, "dir"); ok {
			switch toLowerASCII(strings.TrimSpace(v)) {
			case "rtl":
				return "rtl"
			case "ltr", "auto":
				return "ltr"
			}
		}
	}
	return "ltr"
}
