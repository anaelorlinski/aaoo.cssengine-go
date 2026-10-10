package stylesheet

import (
	"math"
	"slices"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/logger"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// Media queries: media types plus the size and color features of Media
// Queries Level 3 (width, height, aspect-ratio, orientation, color,
// monochrome, color-index, with min-/max- prefixes) and the Level 4 range syntax ((width >= 40em),
// (400px < width <= 700px)). Queries are evaluated against a static
// Device — there is no interactivity or reflow, so a query is decided
// once, at stylesheet preprocessing time.

// Device describes the rendering target that media queries are
// evaluated against.
type Device struct {
	// MediaType is matched against plain media types ("print", "screen",
	// ...); an empty value defaults to "print".
	MediaType string
	// Width and Height are the viewport dimensions in CSS pixels used by
	// size features. When zero, [pr.InitialPageSize()] (A4 portrait)
	// applies.
	Width, Height pr.Fl
}

func (d Device) size() (w, h pr.Fl) {
	if d.Width > 0 && d.Height > 0 {
		return d.Width, d.Height
	}
	return pr.InitialPageSize()[0], pr.InitialPageSize()[1]
}

func (d Device) mediaType() string {
	if d.MediaType == "" {
		return "print"
	}
	return d.MediaType
}

// MediaQuery is one comma-separated alternative of a media query list
// (Media Queries 4 §3, https://drafts.csswg.org/mediaqueries-4/#mq-syntax):
// an optional media type, possibly negated, and a condition.
type MediaQuery struct {
	mediaType string     // lowercase; "" or "all" match any device
	negated   bool       // "not <media-type> …": negates the whole query
	cond      *mediaExpr // nil for none
	// invalid marks a query with bad syntax: per spec it evaluates to "not
	// all" (never matches, even under "not"), while the other alternatives
	// of the list still apply.
	invalid bool
}

// mediaExpr is a media condition: a feature test, the negation, the
// conjunction or the disjunction of conditions, or an unknown test
// (<general-enclosed>, an unknown feature or value), which evaluates to
// "unknown".
type mediaExpr struct {
	op       mediaOp
	feature  mediaCond    // op == opFeature
	children []*mediaExpr // op == opNot (one), opAnd, opOr
}

type mediaOp uint8

const (
	opFeature mediaOp = iota
	opUnknown
	opNot
	opAnd
	opOr
)

// tri is the three-valued logic of media conditions (Media Queries 4 §3.3).
type tri uint8

const (
	triFalse tri = iota
	triTrue
	triUnknown
)

func toTri(b bool) tri {
	if b {
		return triTrue
	}
	return triFalse
}

func (e *mediaExpr) eval(device Device) tri {
	switch e.op {
	case opFeature:
		return toTri(e.feature.eval(device))
	case opNot:
		switch v := e.children[0].eval(device); v {
		case triTrue:
			return triFalse
		case triFalse:
			return triTrue
		default:
			return v
		}
	case opAnd:
		out := triTrue
		for _, c := range e.children {
			switch c.eval(device) {
			case triFalse:
				return triFalse
			case triUnknown:
				out = triUnknown
			}
		}
		return out
	case opOr:
		out := triFalse
		for _, c := range e.children {
			switch c.eval(device) {
			case triTrue:
				return triTrue
			case triUnknown:
				out = triUnknown
			}
		}
		return out
	default:
		return triUnknown
	}
}

// dependsOnSize reports whether e tests a size feature.
func (e *mediaExpr) dependsOnSize() bool {
	if e.op == opFeature {
		switch e.feature.feature {
		case "width", "height", "aspect-ratio", "orientation":
			return true
		}
	}
	for _, c := range e.children {
		if c.dependsOnSize() {
			return true
		}
	}
	return false
}

// mediaCond is one parenthesized feature test.
type mediaCond struct {
	feature      string // "width", "height", "aspect-ratio", "orientation", "color", "monochrome", "color-index"
	orientation  string // "portrait" or "landscape" (feature == "orientation")
	keyword      string // the value of a discrete feature (see discreteFeatures)
	lo, hi       pr.Fl
	hasLo, hasHi bool
	loIncl       bool
	hiIncl       bool
	boolean      bool // bare (width) form: true when the value is non-zero
}

// discreteFeatures are the media features taking keywords, with their value
// on a static paged device (Media Queries 4 §5–§7, 5 §11 user preferences):
// no scripting, no update, no pointing device, paged, sRGB, light, no
// preference. The first keyword of values is the device's.
var discreteFeatures = map[string][]string{
	"scripting":                    {"none", "initial-only", "enabled"},
	"update":                       {"none", "slow", "fast"},
	"hover":                        {"none", "hover"},
	"any-hover":                    {"none", "hover"},
	"pointer":                      {"none", "coarse", "fine"},
	"any-pointer":                  {"none", "coarse", "fine"},
	"overflow-block":               {"paged", "none", "scroll", "optional-paged"},
	"overflow-inline":              {"none", "scroll"},
	"color-gamut":                  {"srgb", "p3", "rec2020"},
	"dynamic-range":                {"standard", "high"},
	"video-dynamic-range":          {"standard", "high"},
	"prefers-color-scheme":         {"light", "dark"},
	"prefers-reduced-motion":       {"no-preference", "reduce"},
	"prefers-reduced-transparency": {"no-preference", "reduce"},
	"prefers-reduced-data":         {"no-preference", "reduce"},
	"prefers-contrast":             {"no-preference", "less", "more", "custom"},
	"forced-colors":                {"none", "active"},
	"inverted-colors":              {"none", "inverted"},
}

// falseInBooleanContext are the keywords for which a feature evaluated
// alone, as (hover), is false (Media Queries 4 §2.4.1).
var falseInBooleanContext = map[string]bool{"none": true, "no-preference": true}

// deviceResolution is the resolution of the device, in dppx: CSS pixels are
// the device pixels.
const deviceResolution = 1

// deviceColorBits is the number of bits per color component of the
// output, for the "color" media feature.
const deviceColorBits = 8

func (c mediaCond) eval(device Device) bool {
	w, h := device.size()

	if values, ok := discreteFeatures[c.feature]; ok {
		if c.boolean {
			return !falseInBooleanContext[values[0]]
		}
		return c.keyword == values[0]
	}

	if c.feature == "orientation" {
		portrait := h >= w
		switch c.orientation {
		case "portrait":
			return portrait
		case "landscape":
			return !portrait
		}
		return c.boolean // bare (orientation)
	}

	var value pr.Fl
	switch c.feature {
	case "width":
		value = w
	case "height":
		value = h
	case "aspect-ratio":
		value = w / h
	case "color":
		value = deviceColorBits
	case "resolution":
		value = deviceResolution
	case "grid":
		value = 0 // a bitmap device
	case "monochrome", "color-index":
		// a color device, without a color lookup table
		value = 0
	default:
		return false
	}

	if c.boolean {
		return value > 0
	}
	// a ratio is a quotient: within rounding, 210/297 equals the ratio of an
	// A4 device
	equal := func(a, b pr.Fl) bool { return a == b }
	if c.feature == "aspect-ratio" {
		value = pr.Fl(float64(w) / float64(h))
		equal = func(a, b pr.Fl) bool { return math.Abs(float64(a)-float64(b)) <= 1e-6*math.Abs(float64(b)) }
	}
	if c.hasLo && !(value > c.lo && !equal(value, c.lo) || c.loIncl && equal(value, c.lo)) {
		return false
	}
	if c.hasHi && !(value < c.hi && !equal(value, c.hi) || c.hiIncl && equal(value, c.hi)) {
		return false
	}
	return true
}

func (q MediaQuery) eval(device Device) bool {
	if q.invalid {
		return false
	}
	result := toTri(q.mediaType == "" || q.mediaType == "all" || q.mediaType == device.mediaType())
	if q.cond != nil && result == triTrue {
		result = q.cond.eval(device)
	}
	if q.negated {
		switch result {
		case triTrue:
			result = triFalse
		case triFalse:
			result = triTrue
		}
	}
	// an unknown result is false
	return result == triTrue
}

// EvaluateMediaQuery returns whether at least one alternative of the
// query list matches the device.
func EvaluateMediaQuery(queryList []MediaQuery, device Device) bool {
	for _, query := range queryList {
		if query.eval(device) {
			return true
		}
	}
	return false
}

// DependsOnSize reports whether the result of the query list may depend on
// the size of the device: whether it tests width, height, aspect-ratio or
// orientation.
func DependsOnSize(queryList []MediaQuery) bool {
	for _, query := range queryList {
		if query.cond != nil && query.cond.dependsOnSize() {
			return true
		}
	}
	return false
}

// ParseMediaQueryString parses a media query list given as a plain
// string (the media="" attribute of <style> and <link>).
func ParseMediaQueryString(s string) []MediaQuery {
	return ParseMediaQuery(parser.Tokenize([]byte(s), true))
}

// ParseMediaQuery parses the token form of a media query list (the
// prelude of @media, or the tail of @import). An empty list means
// "all". A malformed alternative becomes an invalid (never-matching)
// query without discarding its siblings.
func ParseMediaQuery(tokens []Token) []MediaQuery {
	if len(parser.RemoveWhitespace(tokens)) == 0 {
		return []MediaQuery{{}} // "all"
	}
	var out []MediaQuery
	for _, part := range parser.SplitOnComma(tokens) {
		q := parseOneMediaQuery(parser.RemoveWhitespace(part))
		if q.invalid {
			logger.L().Warn("Invalid or unsupported media query", "query", strings.TrimSpace(parser.Serialize(part)))
		}
		out = append(out, q)
	}
	return out
}

func invalidQuery([]Token) MediaQuery { return MediaQuery{invalid: true} }

// parseOneMediaQuery parses
//
//	<media-query> = <media-condition>
//	              | [ not | only ]? <media-type> [ and <media-condition-without-or> ]?
func parseOneMediaQuery(part []Token) MediaQuery {
	if len(part) == 0 { // an empty item of a list is "not all"
		return invalidQuery(part)
	}
	first, isIdent := asIdent(part, 0)
	if !isIdent || first == "not" && len(part) > 1 && !isMediaType(part[1]) {
		// a media condition, or-ed conditions allowed
		cond, ok := parseMediaCondition(part, true)
		if !ok {
			return invalidQuery(part)
		}
		return MediaQuery{cond: cond}
	}

	var q MediaQuery
	i := 0
	if first == "not" || first == "only" {
		q.negated = first == "not"
		i++
	}
	if i >= len(part) || !isMediaType(part[i]) {
		return invalidQuery(part)
	}
	q.mediaType, _ = asIdent(part, i)
	i++
	if i < len(part) {
		if and, _ := asIdent(part, i); and != "and" || i+1 >= len(part) {
			return invalidQuery(part)
		}
		cond, ok := parseMediaCondition(part[i+1:], false)
		if !ok {
			return invalidQuery(part)
		}
		q.cond = cond
	}
	return q
}

// isMediaType reports whether token is a <media-type>: an identifier other
// than the reserved only, not, and, or and layer.
func isMediaType(token Token) bool {
	ident, ok := asIdent([]Token{token}, 0)
	switch ident {
	case "only", "not", "and", "or", "layer":
		return false
	}
	return ok
}

// parseMediaCondition parses a <media-condition> (allowOr) or a
// <media-condition-without-or>, from tokens without whitespace:
//
//	<media-not> | <media-in-parens> [ [ and <media-in-parens> ]* | [ or <media-in-parens> ]* ]
func parseMediaCondition(tokens []Token, allowOr bool) (*mediaExpr, bool) {
	if len(tokens) == 0 {
		return nil, false
	}
	if not, _ := asIdent(tokens, 0); not == "not" {
		if len(tokens) != 2 {
			return nil, false
		}
		inner, ok := parseMediaInParens(tokens[1])
		if !ok {
			return nil, false
		}
		return &mediaExpr{op: opNot, children: []*mediaExpr{inner}}, true
	}
	first, ok := parseMediaInParens(tokens[0])
	if !ok {
		return nil, false
	}
	if len(tokens) == 1 {
		return first, true
	}
	out := &mediaExpr{children: []*mediaExpr{first}}
	connective := ""
	for i := 1; i < len(tokens); i += 2 {
		ident, _ := asIdent(tokens, i)
		switch {
		case ident != "and" && ident != "or",
			ident == "or" && !allowOr,
			connective != "" && ident != connective, // "and" and "or" do not mix
			i+1 >= len(tokens):
			return nil, false
		}
		connective = ident
		next, ok := parseMediaInParens(tokens[i+1])
		if !ok {
			return nil, false
		}
		out.children = append(out.children, next)
	}
	out.op = opAnd
	if connective == "or" {
		out.op = opOr
	}
	return out, true
}

// parseMediaInParens parses
//
//	<media-in-parens> = ( <media-condition> ) | ( <media-feature> ) | <general-enclosed>
//
// A <general-enclosed>, an unknown feature or a value invalid for the
// feature evaluate to "unknown".
func parseMediaInParens(token Token) (*mediaExpr, bool) {
	switch token := token.(type) {
	case parser.ParenthesesBlock:
		args := parser.RemoveWhitespace(token.Arguments)
		if cond, ok := parseMediaCondition(args, true); ok {
			return cond, true
		}
		if feature, ok := parseMediaCond(args); ok {
			return &mediaExpr{op: opFeature, feature: feature}, true
		}
		return &mediaExpr{op: opUnknown}, true
	case parser.FunctionBlock:
		return &mediaExpr{op: opUnknown}, true
	default:
		return nil, false
	}
}

func asIdent(tokens []Token, i int) (string, bool) {
	if i < len(tokens) {
		if ident, ok := tokens[i].(parser.Ident); ok {
			return utils.AsciiLower(ident.Value), true
		}
	}
	return "", false
}

// canonicalFeature maps a feature name to the value it tests, or
// returns false for features we do not support.
func canonicalFeature(name string) (string, bool) {
	switch name {
	case "width", "device-width":
		return "width", true
	case "height", "device-height":
		return "height", true
	case "aspect-ratio", "device-aspect-ratio":
		return "aspect-ratio", true
	case "orientation":
		return "orientation", true
	case "color", "monochrome", "color-index", "resolution", "grid":
		return name, true
	}
	if _, ok := discreteFeatures[name]; ok {
		return name, true
	}
	return "", false
}

// parseMediaCond parses the inside of one parenthesized feature, in
// either the colon form (min-width: 10cm) or the range form
// (400px <= width < 700px).
func parseMediaCond(args []Token) (mediaCond, bool) {
	if len(args) == 0 {
		return mediaCond{}, false
	}

	// colon form: ident [ ':' value... ]
	if name, ok := asIdent(args, 0); ok {
		if len(args) == 1 {
			feature, known := canonicalFeature(name)
			if !known {
				return mediaCond{}, false
			}
			return mediaCond{feature: feature, boolean: true}, true
		}
		if isDelim(args[1], ":") {
			return parseColonCond(name, args[2:])
		}
	}
	return parseRangeCond(args)
}

func parseColonCond(name string, value []Token) (mediaCond, bool) {
	var minP, maxP bool
	if cut, ok := strings.CutPrefix(name, "min-"); ok {
		name, minP = cut, true
	} else if cut, ok := strings.CutPrefix(name, "max-"); ok {
		name, maxP = cut, true
	}
	feature, known := canonicalFeature(name)
	if !known {
		return mediaCond{}, false
	}

	if values, ok := discreteFeatures[feature]; ok {
		if minP || maxP {
			return mediaCond{}, false
		}
		k, ok := asIdent(value, 0)
		if !ok || len(value) != 1 || !slices.Contains(values, k) {
			return mediaCond{}, false // an unknown value: "unknown"
		}
		return mediaCond{feature: feature, keyword: k}, true
	}
	if feature == "orientation" {
		if minP || maxP {
			return mediaCond{}, false
		}
		o, ok := asIdent(value, 0)
		if !ok || len(value) != 1 || (o != "portrait" && o != "landscape") {
			return mediaCond{}, false
		}
		return mediaCond{feature: feature, orientation: o}, true
	}

	v, ok := parseFeatureValue(feature, value)
	if !ok {
		return mediaCond{}, false
	}
	cond := mediaCond{feature: feature}
	switch {
	case minP:
		cond.lo, cond.hasLo, cond.loIncl = v, true, true
	case maxP:
		cond.hi, cond.hasHi, cond.hiIncl = v, true, true
	default: // exact
		cond.lo, cond.hasLo, cond.loIncl = v, true, true
		cond.hi, cond.hasHi, cond.hiIncl = v, true, true
	}
	return cond, true
}

// parseRangeCond parses the level 4 range syntax:
//
//	width > 400px | 400px <= width | 400px <= width < 700px
func parseRangeCond(args []Token) (mediaCond, bool) {
	// locate the feature name
	nameIdx := -1
	var feature string
	for i := range args {
		if name, ok := asIdent(args, i); ok {
			if f, known := canonicalFeature(name); known {
				nameIdx, feature = i, f
				break
			}
		}
	}
	if nameIdx < 0 || feature == "orientation" {
		return mediaCond{}, false
	}

	cond := mediaCond{feature: feature}

	// left side: value op NAME — the op may be two tokens ("<="), so try
	// matching it from two positions before the name first
	if nameIdx > 0 {
		op, n := cmpOp(""), 0
		if nameIdx >= 2 {
			if o, k := parseCmpOp(args, nameIdx-2); k == 2 {
				op, n = o, k
			}
		}
		if n == 0 {
			if o, k := parseCmpOp(args, nameIdx-1); k == 1 {
				op, n = o, k
			}
		}
		if n == 0 {
			return mediaCond{}, false
		}
		v, ok := parseFeatureValue(feature, args[:nameIdx-n])
		if !ok || !applyBound(&cond, flipOp(op), v) {
			return mediaCond{}, false
		}
	}
	// right side: NAME op value
	if nameIdx < len(args)-1 {
		op, n := parseCmpOp(args, nameIdx+1)
		if n == 0 || nameIdx+1+n >= len(args) {
			return mediaCond{}, false
		}
		v, ok := parseFeatureValue(feature, args[nameIdx+1+n:])
		if !ok || !applyBound(&cond, op, v) {
			return mediaCond{}, false
		}
	}
	if !cond.hasLo && !cond.hasHi {
		return mediaCond{}, false
	}
	return cond, true
}

// cmpOp encodes how the feature value compares to the literal:
// "<" means feature < value, ">=" means feature >= value, etc.
type cmpOp string

// parseCmpOp reads a comparison operator at index i, returning it and
// the number of tokens consumed (0 when there is none). "<=" and ">="
// arrive from the tokenizer as two adjacent delim tokens.
func parseCmpOp(args []Token, i int) (cmpOp, int) {
	head := ""
	switch {
	case isDelim(args[i], "<"):
		head = "<"
	case isDelim(args[i], ">"):
		head = ">"
	case isDelim(args[i], "="):
		return "=", 1
	default:
		return "", 0
	}
	// "<=" and ">=" are two adjacent tokens: "< =" is not an operator
	if i+1 < len(args) && isDelim(args[i+1], "=") && adjacent(args[i], args[i+1]) {
		return cmpOp(head + "="), 2
	}
	return cmpOp(head), 1
}

func flipOp(op cmpOp) cmpOp {
	switch op {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	}
	return op
}

// applyBound records "feature op v" on cond, refusing conflicting
// bounds (two lower bounds, two upper bounds).
func applyBound(cond *mediaCond, op cmpOp, v pr.Fl) bool {
	switch op {
	case ">", ">=":
		if cond.hasLo {
			return false
		}
		cond.lo, cond.hasLo, cond.loIncl = v, true, op == ">="
	case "<", "<=":
		if cond.hasHi {
			return false
		}
		cond.hi, cond.hasHi, cond.hiIncl = v, true, op == "<="
	case "=":
		if cond.hasLo || cond.hasHi {
			return false
		}
		cond.lo, cond.hasLo, cond.loIncl = v, true, true
		cond.hi, cond.hasHi, cond.hiIncl = v, true, true
	default:
		return false
	}
	return true
}

// mediaLengthsToPixels converts the units usable in media query lengths.
// Font-relative units resolve against the initial font size (16px), per
// the spec: media queries never depend on the document's own styles.
var mediaLengthsToPixels = map[string]pr.Fl{
	"px": 1, "pt": 96. / 72., "pc": 16, "in": 96,
	"cm": 96. / 2.54, "mm": 96. / 25.4, "q": 96. / 25.4 / 4.,
	"em": 16, "rem": 16,
}

// resolutionUnits converts the <resolution> units to dppx.
var resolutionUnits = map[string]pr.Fl{"dppx": 1, "x": 1, "dpi": 1. / 96, "dpcm": 2.54 / 96}

// parseFeatureValue parses a length (width/height) or a ratio
// (aspect-ratio) from the given value tokens.
func parseFeatureValue(feature string, value []Token) (pr.Fl, bool) {
	if len(value) == 0 {
		return 0, false
	}
	if feature == "aspect-ratio" {
		a, ok := value[0].(parser.Number)
		if !ok || a.ValueF <= 0 {
			return 0, false
		}
		if len(value) == 1 {
			return pr.Fl(a.ValueF), true
		}
		if len(value) == 3 && isDelim(value[1], "/") {
			if b, ok := value[2].(parser.Number); ok && b.ValueF > 0 {
				return pr.Fl(a.ValueF) / pr.Fl(b.ValueF), true
			}
		}
		return 0, false
	}

	if len(value) != 1 {
		return 0, false
	}
	if feature == "resolution" {
		if d, ok := value[0].(parser.Dimension); ok {
			if factor, ok := resolutionUnits[utils.AsciiLower(d.Unit)]; ok && d.ValueF >= 0 {
				return pr.Fl(d.ValueF) * factor, true
			}
		}
		return 0, false
	}
	if feature == "color" || feature == "monochrome" || feature == "color-index" || feature == "grid" {
		// <integer>
		if v, ok := value[0].(parser.Number); ok && v.IsInt() && v.ValueF >= 0 {
			return pr.Fl(v.ValueF), true
		}
		return 0, false
	}
	switch v := value[0].(type) {
	case parser.Dimension:
		if factor, ok := mediaLengthsToPixels[utils.AsciiLower(v.Unit)]; ok {
			return pr.Fl(v.ValueF) * factor, true
		}
	case parser.Number:
		if v.ValueF == 0 {
			return 0, true
		}
	}
	return 0, false
}

func isDelim(token Token, value string) bool {
	lit, ok := token.(parser.Literal)
	return ok && lit.Value == value
}

// adjacent reports whether b immediately follows the one-character token a.
func adjacent(a, b Token) bool {
	pa, pb := a.Pos(), b.Pos()
	return pa.Line == pb.Line && pa.Column+1 == pb.Column
}
