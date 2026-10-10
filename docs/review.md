# Review record

What has been reviewed in this engine, and in the parts of webrender that
drive it (`html/tree`, `svg`, `images`), across four reviews between
2026-10-09 and 2026-10-10. It has two uses:

- **Looking for new issues**: skip what is listed as covered, or go deeper
  than the depth noted; start from the gaps at the end of each area.
- **Revalidating**: each area lists what to run and the hostile inputs to
  try again, so that a general check redoes what the reviews did.

The detailed findings (repro, line numbers at the time) are in webrender's
`docs/`: `css-review-bugs.md` (review 1, IDs P, V, SEL, CAS, C, R, G, PR,
T), `svg-review-bugs.md` (SVG1–2), and `review-2026-10-09.md` (reviews 2 to
4, IDs PC/SC/CS/VP/SV/MOD, then `…2-n`, then `…3-n`). Every finding was
reproduced by running code; every fix has a test.

| Review | Date | Scope | Found | Status |
|---|---|---|---|---|
| 1 | 2026-10-09 | `css/*` and `html/tree` (before extraction), svg imports | 65 + 9 found while fixing | all fixed |
| 2 | 2026-10-09 | the extracted module, six areas | 54 | fixed; MOD3, MOD7 decided before v0.1.0; CS11 not a bug |
| 3 | 2026-10-10 | verify review 2, regressions, gaps | 50 | fixed; P2-2 and part of V2-7 not defects |
| 4 | 2026-10-10 | verify review 3, regressions, gaps | 45 | fixed; two kept by decision (below) |

Severity, in all of them: S1 crash, hang, exponential cost, security; S2
wrong output in common use or an API hazard; S3 edge case; S4 quality.

## How to revalidate everything

```sh
# engine
cd aaoo.cssengine-go
gofmt -l . ; goimports -l .
go vet ./... && go test ./... && go test -race ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...   # staticcheck.conf: all but ST1003, ST1016
go generate ./properties/          # must leave properties/*_gen.go unchanged (git diff)
go test -run xxx -fuzz FuzzTokenize ./parser                   # and the other Fuzz* targets
# consumers
cd ../webrender && go vet ./... && go test ./... && go test -race ./svg ./html/tree ./images
cd svgtests && go test .            # resvg suite against baseline.csv: no regression allowed
cd ../../.. && go test . ./internal/...   # compositor: 34 known failures in internal/drawtest, no new ones
```

The timing assertions scale by 10 under `-race` (`timeScale`, in
`race_on_test.go` / `race_off_test.go`).

## Limits

All resource bounds are constants in `limits` (this module) and
`webrender/limits`, each with its reason: nesting 512, math depth 64,
imports 16 deep / 256 total, substituted tokens 64K, counter representation
60, nth coefficient 2³¹−1, lengths ±2²⁵; `<use>` 100,000 elements / 64
documents / 16 deep, reference depth 16, draws max(1,000,000, 100 ×
elements), pattern tile 20 draws, nested images 256 / 8 deep, 8 SVG size
variants, 8 layout passes.

- Covered: every construct whose cost can grow faster than its source
  (review 1 T5/T6/C3/P2, review 2 SV1/SV2, review 3 G2-1/G2-6, review 4
  SV3-1/SV3-2/SV3-9). Values checked against legit large documents
  (Wikimedia world maps up to 6,374 elements, US counties, Ghostscript
  tiger, Paris metro) and the 1,722 resvg files: no limit reached, at most
  4.2 draws per element.
- Revalidate: grep for numeric caps outside `limits` (none should exist
  besides log truncation and the gamut bisection bound); rerun the
  amplification tests (`TestUseExpansionBounded`, `TestUseExternalCycle`,
  `TestUseSpriteSheet`, `TestDrawAmplificationBounded`,
  `TestMarkersStopWithBudget`, `TestImageFanOut`, `TestVariableExponentialExpansion`).

## parser (tokenizer, parser, serializer)

- Reviewed in depth: tinycss2 port against its corpus; fuzzing (no panic);
  nesting depth; linear passes on adversarial inputs (11 shapes, 2K–16K
  tokens); declarations (run to `;`, `!important` as the last two
  significant tokens, nested `a: {b} !important`, top-level `--x:` preludes);
  unicode-range boundary (a range only ends at a token boundary; `u+a`
  selectors still match as `u + a`, P2-2 decided); BOM; bad-url remnants;
  the serializer round trip (3,600 token pairs, review 4: the missing pairs
  `#`+`-->`, range+url, range+range, `u`+`+hex` fixed).
- Fixed: P1–P14, PC1–PC2, PC8–PC11, P2-6..P2-9, PA3-4.
- Clean: every pass linear, `Tokenize`/`Serialize` round trip, positions.
- Revalidate: `FuzzTokenize`, `TestSerializeBadPairs3`,
  `TestTokenizeDeepNesting`, `TestParseBlocksContentsManyNestedRules`, the
  css-parsing-tests corpus.
- Not looked at closely: performance constants (four input copies,
  per-token regexps) — measured linear, not optimised.

## color, and `<color>` in parser and validation

- Reviewed in depth: CSS Color 4/5 grammars (legacy and modern, mixing,
  clamping, `none`, hex, hue units), 14 colour spaces (round trip within
  4e-14), gamut mapping (NaN, ±∞, huge chroma, denormals; bisection
  bounded), `color-mix()` (all spaces, the four hue methods, percentage
  normalisation, premultiplied alpha, missing components),
  `light-dark()`, `contrast-color()`, `device-cmyk()`, `color(--profile)`,
  relative colours (`rgb/hsl/hwb/lab/lch/oklab/oklch(from …)`, math in
  channels, angle math in hues, nesting in `color-mix()`/`light-dark()`),
  colours depending on `currentcolor` (`ColorDependsOnCurrent`,
  `Color.Resolve`, `ParseColorValueWith`, `ParseColorValueNested`).
- Fixed: P6–P10, PC3–PC7, PC12, P2-1, P2-3..P2-5, P2-10, V2-2, PA3-1..PA3-6,
  VA3-2, VA3-4, VA3-8.
- Clean: every `ColorType` switch in both repos handles
  `ColorDependsOnCurrent`; no `==` or map key on `pr.Color` outside tests.
- Revalidate: `TestColorCorpora` (8,061 tinycss2 cases), `TestColorWPTLab`,
  `TestColorDependsOnCurrent`, `TestColorMixReview3`,
  `TestRelativeColorsReview3`, `color/gamut_extreme_test.go`; webrender
  `TestCurrentColorInFunctions`, `TestCurrentColorBorderImage`.
- Known limits: deferred colours compare by identity (documented on
  `Color`); profile colours are not renderable (`NewColor` refuses them).

## selector

- Reviewed in depth: matching cost (descendant chains, nested
  `:is/:where/:not/:has/:nth-child(of)`, deep 100K, wide 50K, bushy 38K
  documents, with and without a `Cache`); relative selectors (`:has()`
  left to right, memoised per document); the `Cache` (positions, `:has`
  results, forward chains, chain memo, HTML state scans) and the private
  cache of a single `Match`; parsing (forgiving lists, `skipListItem`,
  identifiers, `an+b`, attribute flags and namespaces, pseudo-element
  placement, depth limit, `ParseGroupStrict`); serialization (CSSOM
  escaping, every accepted form round-trips); specificity; the HTML state
  pseudo-classes against HTML §4.16.3 (`:any-link`, `:scope`, `:defined`,
  `:open`, `:required/:optional`, `:read-only/:read-write`,
  `:placeholder-shown`, `:default`, `:indeterminate`, `:valid/:invalid`
  with selectedness, placeholder label options, radio groups, the `form`
  attribute, candidates for constraint validation, `:dir()`); the static
  never-matching ones (interaction, playback, `:host`, `:state()`).
- Fixed: SEL1–SEL9, SC1–SC9, S2-1..S2-12, SE3-1..SE3-13.
- Clean: a differential fuzzer against a naive matcher (about 280,000
  cases, shared and private caches) finds no wrong result.
- Decided: `:link`/`:any-link` match `<link href>` (W3C selectors suite,
  browsers), though HTML lists only `a` and `area`.
- Revalidate: `TestDifferential`, `TestDifferentialExtended`,
  `TestNestedLogicalCallCount`, `TestNthLinear`, `TestHasSiblingLinear`,
  `TestMatchWithoutCacheCost`, `TestMatchWithNestedAllocations`,
  `TestHTMLStatesReview3`, `TestHTMLStatesCost`, the W3C/cssselect2
  fixtures (`TestValidSelectors`, `TestShakespeare`).
- Not covered: non-standard extensions (`:contains`, `:matches`,
  `:haschild`, `!=`, `#=`) beyond the fixtures; `:nth-child(of)` combined
  with every state pseudo-class.

## cascade (matcher, index, layers)

- Reviewed in depth: the rule index (equivalent to testing every rule),
  `MatchWith` candidate order and stable sort, layers (named, dotted,
  nested, anonymous, statements before `@import`, `layer()` imports, order
  across sheets of an origin, `!important` reversal), layer name parsing,
  cost of many layers (indexed by name since review 4), concurrent use of a
  shared `Matcher`.
- Fixed: CAS1–CAS5, SE3-7.
- Revalidate: `TestMatchIndexEquivalence`, `TestLayer*`, `TestManyLayers`,
  `-race`.

## stylesheet (walk, @import, @media, @supports)

- Reviewed in depth: Media Queries 4 (types, `only`/`not`, three-valued
  logic at every nesting, range syntax on both sides, units, A4 rounding,
  `<general-enclosed>`, print's discrete features, `aspect-ratio` with an
  epsilon, resolution), `@supports` (declarations, `!important`,
  `selector()` at any depth and strict on forgiving lists, custom
  properties, `var()` needing a known property, the `all` shorthand),
  `@import` (url forms, `supports()`/`layer`/media order, cycles, diamonds,
  depth and count caps, the import place ended by `@layer` statements),
  `SizeQueries` reporting; hostile input (2000-deep parens, 5000 `and`,
  NUL, 100 KB selectors).
- Fixed: T6, T9, T10, CS1, CS5–CS8, CS11 (part), C2-1..C2-4, ST3-6.
- Decided: `@import … layer(x) screen` on a non-matching medium does not
  declare `x` (the spec text does not settle it; Chrome declares it).
- Revalidate: `stylesheet` tests, `TestSupportsSelector`,
  `TestImportCycle`, `TestWalkImportForms`.

## properties, validation, counters

- Reviewed in depth: the math engine (type checking by probe evaluation,
  NaN/∞, clamps to the property range, depth, trig asymptotes, `round()`
  modes, `sign()`/`atan2()` shapes, like-type division in number
  properties, font-relative units, context-free evaluation of
  `<integer>`/`<number>`/`<angle>`/`<resolution>`), math accepted by every
  validator WeasyPrint accepts (and `TestHostileValuesEveryProperty` reads
  every property back), shorthands (including `all`), CSS-wide keywords,
  `@page` size and bleed, `@font-face` and `@counter-style` descriptors,
  counters at the integer extremes, fallback and `extends` cycles, the
  property registry (built-in names refused, one registration per name,
  sealed when parsing starts, registration from an `init` only), the
  generator, relative `url()`s inside values substituted later
  (`var()`), inherited flags.
- Fixed: V1–V16, C1–C7, R1–R5, G1, PR1, VP3–VP12, V2-1, V2-4..V2-9, M2-1,
  ST3-1, VA3-6, VA3-7, MO3-2, MO3-3.
- Decided: `calc(-10cm)` in `@page size` is clamped (Values 4), not
  invalid; `round(1.5px)` without a step is accepted (WeasyPrint's tests).
- Revalidate: `validation` and `counters` tests (`math_review_test.go`,
  `math_properties_test.go`, `TestMathInMoreProperties`,
  `TestLikeTypeDivisionNumber`, `TestAllShorthand`, `TestURLThroughVar`,
  `TestPageSizeViewportMath`), `FuzzRenderValue`, `properties` registry
  tests.

## Consumers in webrender

These are outside this module but were reviewed with it, since they drive
it; revalidate them with webrender's tests.

- **html/tree cascade**: origins, importance, element-attached styles,
  layers, presentational hints as their own origin, `revert` and
  `revert-layer` (Cascade 5 §7.3, `@page` too, WPT revert-layer-005 and
  -012), custom properties (computed where declared, CSS-wide keywords,
  empty values, cycles, 64K cap, linear at 2000 × 5000), computed values of
  colours (`currentcolor` in functions, shadows, gradient stops,
  `border-image-source`), shared stylesheets across concurrent renders,
  the default stylesheets parsed at init after `svg/svgcss` registered the
  SVG properties. IDs: T1–T17, CS2–CS4, CS9, CS10, VP1, VP2, C2-6,
  ST3-2..ST3-5, VA3-3. Tests: `layers_test.go` (`TestRevert*`),
  `review_fixes_test.go`, `math_units_test.go`, `html/layout`.
- **svg**: `<use>` (budgets, cycles, external documents parsed once,
  instances cascaded again from the `<use>`, own-ancestor references
  refused, symbol sizing), the cascade (specified attributes kept,
  `currentColor` a keyword resolved per node, CSS-wide keywords, geometry
  attributes not inherited), draw budget and reference depth (clip, mask,
  marker, pattern), `@media` against the drawn size and `<style media>`,
  `<style type>`, paint servers with the image's definitions. IDs:
  SVG1–2, SV1–SV9, G2-1..G2-8, SV3-1..SV3-9. Tests: `use_limits_test.go`,
  `use_review3_test.go`, `css_media_test.go`, svgtests (384 perfect of
  1,722).
- **images**: loading as a depth-first walk with loop detection, a budget
  shared by one load, a loader memoised per image. Tests:
  `TestImageReferenceLoops`, `TestImageFanOut`.

## Module

- Reviewed: licences end to end (BSD-2/3 of the ported code, W3C full text
  for `color`, CC0 corpus, cssselect2 fixtures; new files carry Anael
  Orlinski), README and NOTICE, package docs, exported API hazards, the
  generator round trip, `go mod tidy`, imports, dead code.
- Fixed: MOD1–MOD13 (except the decisions below), M2-2..M2-12, MO3-4..MO3-9.
- Decided before v0.1.0: MOD3, the tables are unexported and read
  through functions (`InitialValue`, `InitialStyle.Get…`, `IsInherited`,
  `PropertyByName`, `IsKnownProperty`, `PageSize`, `LengthToPixels`, …):
  nothing outside `properties` can modify them, at the cost of a function
  call the compiler inlines. MOD7, webrender's computed-style interface
  (`ElementStyle`, `TextRatioCache`, `SpecifiedAttributes`) moved to
  `webrender/computedstyle`; `ViewportSizes` and `PendingMath` stay, math
  evaluation needs them; matching stays on `golang.org/x/net/html` nodes,
  the standard Go DOM shared by the HTML and SVG engines, as an interface
  would cost every match.
