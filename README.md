# aaoo.cssengine-go

A CSS engine in Go: parsing, selectors, the cascade and property values. No
layout of its own.

The goal is one engine for both HTML and SVG renderers: selectors match on
`golang.org/x/net/html` trees, which hold HTML and SVG documents alike, and
media queries are evaluated against whatever device the caller describes, a
printed page or an SVG viewport.

```go
rules := parser.ParseStylesheetBytes(css, true, true)  // CSS Syntax 3
sel := selector.MustCompile("li:has(+ li.active)")    // Selectors 4
purple := parser.ParseColorString("color-mix(in oklch, red, blue)")
```

Supported: CSS Syntax 3; Selectors 4 (`:is()`, `:where()`, `:not()`,
`:has()`, `:nth-child(… of S)`, the HTML state pseudo-classes on the static
document); Cascade 5 (origins, `!important`, `@layer`); `@import`, `@media`
(Media Queries 4, range syntax), `@supports` (with `selector()`); `@page`,
`@font-face` and `@counter-style` descriptors; Values 4 math (`calc()`,
`min()`/`max()`/`clamp()`, `round()`/`mod()`/`rem()`, trigonometric and
exponential functions, `sign()`/`abs()`); Color 4/5 (`lab()`, `lch()`,
`oklab()`, `oklch()`, `color()`, `color-mix()`, relative colors,
`light-dark()`, `contrast-color()`, `device-cmyk()`); custom properties,
`var()` being substituted by the caller. Not supported: CSS Nesting,
`@container`, `@scope`, `@property`, `@namespace`.

| Package | |
|---|---|
| `parser` | tokenizer, parser, serializer, `<color>` (Color 4/5) |
| `selector` | Selectors 4 on `golang.org/x/net/html` trees |
| `cascade` | rule matching and Cascade 5 layers |
| `stylesheet` | `@import`, `@media`, `@supports`, `@layer` |
| `properties`, `validation` | property values: declarations to typed values, math |
| `counters` | Counter Styles 3 |
| `color` | color spaces and gamut mapping |
| `limits` | every resource cap, in one place |
| `logger` | the `log/slog` logger the engine reports through |

`parser` ports tinycss2, `selector` forks cascadia, and
`properties`/`validation`/`counters` port WeasyPrint. Licenses: `LICENSE`,
the per-package `LICENSE` files and `NOTICE`.
