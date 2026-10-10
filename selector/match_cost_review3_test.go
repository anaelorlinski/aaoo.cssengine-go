package selector

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// Third review, SE3-2: through Match (no document cache) a sibling scan
// costs what a naive scan does, without filling a throw-away table.
func TestMatchWithoutCacheCost(t *testing.T) {
	doc, err := html.Parse(strings.NewReader("<ul>" + strings.Repeat(`<li class="c1"></li>`, 5_000) + `<li class="x"></li></ul>`))
	if err != nil {
		t.Fatal(err)
	}
	items := QueryAll(doc, MustCompile("li"))
	for _, input := range []string{"li:has(~ .x)", "li:nth-child(2n of .c1)", "li:nth-last-child(2n+1)"} {
		sel := MustCompile(input)
		start := time.Now()
		for _, li := range items {
			sel.Match(li)
		}
		if d := time.Since(start); d > 2*timeScale*time.Second {
			t.Errorf("%s through Match: %s for %d items", input, d, len(items))
		}
	}
}

// Third review, SE3-11: under MatchWith, a nested selector not using the
// cache memoizes in the document cache, without a private one per match.
func TestMatchWithNestedAllocations(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`<div class="a"><p>x</p></div>`))
	if err != nil {
		t.Fatal(err)
	}
	p := Query(doc, MustCompile("p"))
	cache := NewCache()
	for _, input := range []string{":is(.a p)", "div :not(div p)"} {
		sel := MustCompile(input)[0]
		if allocs := testing.AllocsPerRun(100, func() { MatchWith(sel, p, cache) }); allocs > 1 {
			t.Errorf("%s: %g allocations per match", input, allocs)
		}
	}
}

// Third review, SE3-12: a comment after "of", the descendant combinator
// serialized as one space, "#-" not an id.
func TestParserReview3(t *testing.T) {
	if _, err := ParseGroup(":nth-child(2n+1 of/**/a)"); err != nil {
		t.Errorf("of/**/: %s", err)
	}
	if got := MustCompile(":where(a b)").String(); got != ":where(a b)" {
		t.Errorf("descendant serialized as %q", got)
	}
	for _, input := range []string{"#-", "#- a"} {
		if _, err := ParseGroup(input); err == nil {
			t.Errorf("%s should be invalid", input)
		}
	}
	for _, input := range []string{"#--", "#-a", "#-\\31 "} {
		if _, err := ParseGroup(input); err != nil {
			t.Errorf("%s: %s", input, err)
		}
	}
}
