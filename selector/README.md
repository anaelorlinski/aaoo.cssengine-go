# selector

The selector package implements CSS selectors for use with the parse trees produced by the html package.

It is based on andybalholm/cascadia commit 8919e381b2b9868b86bb29a833d893796a188f1f, with some API simplification.

Upstream fixes have been backported up to cascadia v1.3.5 (commit d561140466569a34aaf4c86630d9794cef276c57):
selector nesting limit (caa0b27), empty attribute values (6d90623), `\0` escapes (d561140),
`:empty` allocations (9c17a91) and relative `:has()` (8b5adc2, with the `:not` split of 154416a).
Relative `:has()` differs from upstream: the leftmost compound is anchored at the `:has()` element,
as Selectors 4 requires. `backport_test.go` covers these changes.

Fixes from the CSS review (`review_test.go`, `review_count_test.go`):
complex selectors are matched right to left with a per-match memo, so descendant and `~` combinators are
no longer exponential (the same memo is shared by all candidates of a `:has()` search); type and attribute
names are case-insensitive only on HTML elements (SVG keeps `linearGradient`, `viewBox`); namespace syntax
(`*|*`, `svg|rect`, `[xlink|href]`) with a fixed prefix table, since `@namespace` is not supported, and
unprefixed attribute selectors match attributes in no namespace; `:lang()` uses the nearest language,
ASCII case-insensitively, and accepts a list; the `s` attribute flag; `:nth-child(An+B of S)`; ASCII (not
Unicode) case folding for the `i` flag; an+b coefficients are clamped to 32 bits.
