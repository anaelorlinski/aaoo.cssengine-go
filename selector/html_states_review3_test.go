package selector

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

func matchedIDs(t *testing.T, doc string, selector string) string {
	t.Helper()
	root := MustParseHTML(doc)
	sel := MustCompile(selector)
	cache := NewCache()
	var got []string
	walkElements(root, func(e *html.Node) {
		if MatchWith(sel[0], e, cache) {
			if id, ok := attr(e, "id"); ok {
				got = append(got, id)
			}
		}
	})
	return strings.Join(got, " ")
}

// Third review: SE3-3..6, SE3-9, SE3-10, SE3-13.
func TestHTMLStatesReview3(t *testing.T) {
	for _, c := range []struct{ doc, selector, want string }{
		// :open
		{`<details id=d open></details><details id=c></details><dialog id=g open></dialog>`, ":open", "d g"},
		// selectedness: the first option of a drop-down select
		{`<select id=s><option id=o1 value=a><option id=o2 value=b></select>`, "option:checked", "o1"},
		{`<select id=s><option id=o1 selected><option id=o2 selected></select>`, "option:checked", "o2"},
		{`<select multiple><option id=o1><option id=o2 selected></select>`, "option:checked", "o2"},
		{`<select size=3><option id=o1><option id=o2></select>`, "option:checked", ""},
		// select:invalid: implicit selection, placeholder label option
		{`<select id=s required><option value=a></select>`, "select:invalid", ""},
		{`<select id=s required><option value="">-<option value=a></select>`, "select:invalid", "s"},
		{`<select id=s required multiple><option value="" selected></select>`, "select:invalid", ""},
		{`<select id=s required><option selected></option></select>`, "select:invalid", "s"},
		// candidates for constraint validation
		{`<input id=i disabled required><form><button id=b></button><datalist><input id=l required></datalist></form>`, ":valid", "b"},
		{`<input id=i disabled required><form><button id=b></button><datalist><input id=l required></datalist></form>`, ":invalid", ""},
		// radio groups: required by any member, checked by any
		{`<form id=f><input type=radio name=r id=a required><input type=radio name=r id=b></form>`, "input:invalid", "a b"},
		{`<form id=f><input type=radio name=r id=a required><input type=radio name=r id=b checked></form>`, "input:invalid", ""},
		// the form attribute
		{`<form id=f></form><input type=radio name=r id=a form=f><form><input type=radio name=r id=b checked form=f></form>`, ":indeterminate", ""},
		{`<form id=f><button id=x type=button></button></form><button id=b form=f></button>`, ":default", "b"},
		{`<form id=f><input id=i required form=g></form><form id=g></form>`, "form:invalid", "g"},
		// HTML details
		{`<form><button id=b type=foo></button></form>`, ":default", "b"},
		{`<input id=d type=date placeholder=x><input id=t placeholder=x>`, ":placeholder-shown", "t"},
		{`<input id=f type=foo><div contenteditable><input id=c type=checkbox></div>`, ":read-write", "f"},
		// :dir() with another value, :defined, :host, :state()
		{`<p id=p dir=auto></p>`, ":dir(auto)", ""},
		{`<p id=p></p><my-el id=m></my-el>`, ":defined", "p"},
		{`<p id=p></p>`, ":host, :host(p), :state(x), :host-context(p)", ""},
	} {
		if got := matchedIDs(t, c.doc, c.selector); got != c.want {
			t.Errorf("%s in %s: %q, want %q", c.selector, c.doc, got, c.want)
		}
	}
}

// Third review, SE3-1 and SE3-8: radio groups and default buttons are
// computed once per document.
func TestHTMLStatesCost(t *testing.T) {
	var b strings.Builder
	b.WriteString("<form>")
	for i := range 20_000 {
		fmt.Fprintf(&b, `<input type=radio name=r%d>`, i%100)
	}
	for range 5_000 {
		b.WriteString(`<button></button>`)
	}
	b.WriteString("</form>")
	for i := range 20_000 {
		fmt.Fprintf(&b, `<input type=radio name=s%d>`, i%100)
	}
	root := MustParseHTML(b.String())
	start := time.Now()
	for _, sel := range []string{"input:indeterminate", "button:default", "input:invalid"} {
		s := MustCompile(sel)[0]
		cache := NewCache()
		walkElements(root, func(e *html.Node) { MatchWith(s, e, cache) })
	}
	if d := time.Since(start); d > 2*timeScale*time.Second {
		t.Errorf("took %s", d)
	}
}
