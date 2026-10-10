package selector

import (
	"strings"
	"testing"
)

// S2-5: the state pseudo-classes of HTML, and the static never-matching
// ones, are known: they no longer invalidate the rule.
func TestHTMLStatePseudoClasses(t *testing.T) {
	doc := MustParseHTML(`<html dir="rtl"><body>
		<a id="link" href="x">l</a><a id="nolink">n</a>
		<form id="f1"><input id="req" required><input id="opt"><input id="ro" readonly><input id="cb" type="checkbox" checked>
			<input id="ph" placeholder="p"><input id="phv" placeholder="p" value="v"><textarea id="ta"></textarea>
			<button id="b1">1</button><button id="b2">2</button>
			<input type="radio" name="r" id="r1"><input type="radio" name="r" id="r2"></form>
		<form id="f2"><input type="radio" name="r" id="r3" checked><input id="filled" required value="x"></form>
		<progress id="pr"></progress><progress id="prv" value="1"></progress>
		<div contenteditable id="ce"><p id="cep">x</p></div>
		<p id="ltr" dir="ltr">x</p><p id="inherited">y</p>
		<select id="sel" required><option value="">-</option></select></body></html>`)
	for selector, want := range map[string]string{
		":any-link":          "link",
		":required":          "req filled sel",
		":optional":          "opt ro cb ph phv ta r1 r2 r3",
		":read-write":        "req opt ph phv ta filled ce cep",
		":placeholder-shown": "ph",
		":default":           "cb b1 r3",
		":indeterminate":     "r1 r2 pr",
		":invalid":           "f1 req sel",
		":dir(ltr)":          "ltr",
		"input:valid":        "opt cb ph phv r1 r2 r3 filled",
		":scope":             "",
	} {
		var got []string
		for _, e := range QueryAll(doc, MustCompile(selector)) {
			if id, ok := attr(e, "id"); ok {
				got = append(got, id)
			}
		}
		if g := strings.Join(got, " "); g != want {
			t.Errorf("%s: %q, want %q", selector, g, want)
		}
	}
	if !MustCompile(":scope").Match(doc.FirstChild) || !MustCompile(":dir(rtl)").Match(Query(doc, MustCompile("#inherited"))) {
		t.Error(":scope, :dir(rtl)")
	}
	// interaction states never match, and keep the rest of the rule
	for _, input := range []string{"a:hover, a:focus-visible", "div:focus-within", ":target-within", "video:playing", "dialog:modal"} {
		sel, err := ParseGroup(input)
		if err != nil {
			t.Errorf("%s: %s", input, err)
			continue
		}
		if len(QueryAll(doc, sel)) != 0 {
			t.Errorf("%s matches", input)
		}
		if _, err := ParseGroup(sel.String()); err != nil {
			t.Errorf("%s: serialized %q: %s", input, sel.String(), err)
		}
	}
}
