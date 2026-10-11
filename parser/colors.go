package parser

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/color"
	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.gfxcore-go"
)

// Parsing of the CSS <color> type: CSS Color Level 4 and the Level 5
// additions device-cmyk(), color(--profile …), light-dark() and
// contrast-color().
//
// The rules follow tinycss2's color4 and color5 modules, and are checked
// against the css-parsing-tests color corpora.

// RGBA is a color in sRGB, components and alpha in [0, 1]. It is the
// rendered form of a color, shared with the code that draws.
type RGBA = gfxcore.RGBA

type ColorType uint8

const (
	// ColorInvalid is an empty or invalid color specification.
	ColorInvalid ColorType = iota
	// ColorCurrentColor represents the special value "currentColor"
	// which need document context to be resolved.
	ColorCurrentColor
	// ColorRGBA is a specified color: see [Color.Value] and [Color.RGBA].
	ColorRGBA
	// ColorDependsOnCurrent is a color function using currentcolor
	// (color-mix(in srgb, currentcolor, red)), computed for an element by
	// [Color.Resolve].
	ColorDependsOnCurrent
)

// Color is a parsed <color>.
type Color struct {
	Type ColorType
	// Value is the color as specified, in its own color space. It keeps
	// device-cmyk() and wide-gamut colors for backends that can output
	// them. It is the zero Value for colors built from an RGBA only.
	Value color.Value
	// RGBA is the color rendered in sRGB, out-of-gamut colors being
	// gamut-mapped: the form used by sRGB-only backends.
	RGBA RGBA

	// for [ColorDependsOnCurrent]: computes the color from the value of
	// currentcolor. Two such colors compare equal only if they are copies of
	// the same parsed color.
	deferred *deferredColor
}

type deferredColor struct {
	resolve func(current color.Value) (color.Value, ColorType)
}

// NewDeferredColor returns the [ColorDependsOnCurrent] color computed by
// resolve from the value of currentcolor.
func NewDeferredColor(resolve func(current color.Value) (color.Value, ColorType)) Color {
	return Color{Type: ColorDependsOnCurrent, deferred: &deferredColor{resolve: resolve}}
}

// Resolve returns the color for an element whose currentcolor is current
// (the value of its color property): currentcolor is current, and a color
// function using it is computed. Other colors are returned unchanged.
func (c Color) Resolve(current Color) Color {
	switch c.Type {
	case ColorCurrentColor:
		return current
	case ColorDependsOnCurrent:
		if c.deferred == nil { // not built by NewDeferredColor
			return Color{}
		}
		var cur color.Value
		switch {
		case current.Type != ColorRGBA: // no current color: black
			cur = color.New(color.SRGB, 0, 0, 0)
		default:
			cur = current.Value
			// built from an RGBA only, or not convertible (device-cmyk()):
			// its sRGB rendering
			if _, ok := cur.To(color.SRGB); !ok {
				cur = color.New(color.SRGB, float64(current.RGBA.R), float64(current.RGBA.G), float64(current.RGBA.B))
				cur.Alpha = float64(current.RGBA.A)
			}
		}
		if v, typ := c.deferred.resolve(cur); typ == ColorRGBA {
			return NewColor(v)
		}
		return Color{}
	}
	return c
}

func (c Color) IsNone() bool {
	return c.Type == ColorInvalid
}

// NewColor returns the [ColorRGBA] color for v, rendering it in sRGB.
// A color that cannot be rendered (a color profile, which is not
// supported) is invalid.
func NewColor(v color.Value) Color {
	rgb, ok := v.SRGB()
	if !ok {
		return Color{}
	}
	return Color{Type: ColorRGBA, Value: v, RGBA: RGBA{
		R: float32(rgb[0]), G: float32(rgb[1]), B: float32(rgb[2]), A: float32(v.Alpha),
	}}
}

// ColorScheme selects the branch of light-dark().
type ColorScheme uint8

const (
	ColorSchemeLight ColorScheme = iota
	ColorSchemeDark
)

// ParseColorString tokenizes the input before calling [ParseColor].
func ParseColorString(input string) Color {
	return ParseColor(ParseOneComponentValue(Tokenize([]byte(input), true)))
}

// ParseColor parses a <color>, light-dark() resolving to its light branch.
// It returns the zero Color if the token is not a valid color.
func ParseColor(token Token) Color {
	return ParseColorScheme(token, ColorSchemeLight)
}

// ParseColorScheme is like [ParseColor], with the color scheme selecting
// the branch of light-dark().
func ParseColorScheme(token Token, scheme ColorScheme) Color {
	v, typ := ParseColorValue(token, scheme)
	switch typ {
	case ColorCurrentColor:
		return Color{Type: ColorCurrentColor}
	case ColorDependsOnCurrent:
		return NewDeferredColor(func(current color.Value) (color.Value, ColorType) {
			return ParseColorValueWith(token, scheme, &current)
		})
	case ColorRGBA:
		return NewColor(v)
	}
	return Color{}
}

// ParseColorValue parses a <color> without rendering it. Unlike
// [ParseColorScheme], it accepts color(--profile …), whose rendering needs
// the profile.
func ParseColorValue(token Token, scheme ColorScheme) (color.Value, ColorType) {
	return parseColorEnv(token, colorEnv{scheme: scheme})
}

// ParseColorValueWith is [ParseColorValue] with the value currentcolor
// resolves to, inside color functions too. A nil current leaves
// currentcolor as [ColorCurrentColor], and a color function using it as
// [ColorDependsOnCurrent].
func ParseColorValueWith(token Token, scheme ColorScheme, current *color.Value) (color.Value, ColorType) {
	return parseColorEnv(token, colorEnv{scheme: scheme, current: current})
}

// NestedColor parses the colors this package does not know (the relative
// syntax, handled by package validation), given the value of currentcolor
// (nil when unknown). It returns false for the other tokens.
type NestedColor func(token Token, current *color.Value) (color.Value, ColorType, bool)

// ParseColorValueNested is [ParseColorValueWith], nested calls the parser
// for the colors it does not know, at any depth (in color-mix(),
// light-dark(), contrast-color()).
func ParseColorValueNested(token Token, scheme ColorScheme, current *color.Value, nested NestedColor) (color.Value, ColorType) {
	return parseColorEnv(token, colorEnv{scheme: scheme, current: current, nested: nested})
}

// colorEnv is what a <color> depends on: the color scheme (light-dark()),
// the value of currentcolor when known, and the parser of the colors this
// package does not know.
type colorEnv struct {
	scheme  ColorScheme
	current *color.Value
	nested  NestedColor
}

func parseColorEnv(token Token, env colorEnv) (color.Value, ColorType) {
	if env.nested != nil {
		if v, typ, ok := env.nested(token, env.current); ok {
			return v, typ
		}
	}
	if v, typ := parseColor4(token, env); typ != ColorInvalid {
		return v, typ
	}
	return parseColor5(token, env)
}

var colorKeywords = map[string]color.Value{}

func init() {
	for _, table := range [...]map[string][3]uint8{basicColorKeywords, extendedColorKeywords} {
		for name, rgb := range table {
			colorKeywords[name] = color.New(color.SRGB,
				float64(rgb[0])/255, float64(rgb[1])/255, float64(rgb[2])/255)
		}
	}
}

var (
	// (r, g, b) := range 0..255
	basicColorKeywords = map[string][3]uint8{
		"black":   {0, 0, 0},
		"silver":  {192, 192, 192},
		"gray":    {128, 128, 128},
		"white":   {255, 255, 255},
		"maroon":  {128, 0, 0},
		"red":     {255, 0, 0},
		"purple":  {128, 0, 128},
		"fuchsia": {255, 0, 255},
		"green":   {0, 128, 0},
		"lime":    {0, 255, 0},
		"olive":   {128, 128, 0},
		"yellow":  {255, 255, 0},
		"navy":    {0, 0, 128},
		"blue":    {0, 0, 255},
		"teal":    {0, 128, 128},
		"aqua":    {0, 255, 255},
	}

	// (r, g, b) := range 0..255
	extendedColorKeywords = map[string][3]uint8{
		"aliceblue":            {240, 248, 255},
		"antiquewhite":         {250, 235, 215},
		"aqua":                 {0, 255, 255},
		"aquamarine":           {127, 255, 212},
		"azure":                {240, 255, 255},
		"beige":                {245, 245, 220},
		"bisque":               {255, 228, 196},
		"black":                {0, 0, 0},
		"blanchedalmond":       {255, 235, 205},
		"blue":                 {0, 0, 255},
		"blueviolet":           {138, 43, 226},
		"brown":                {165, 42, 42},
		"burlywood":            {222, 184, 135},
		"cadetblue":            {95, 158, 160},
		"chartreuse":           {127, 255, 0},
		"chocolate":            {210, 105, 30},
		"coral":                {255, 127, 80},
		"cornflowerblue":       {100, 149, 237},
		"cornsilk":             {255, 248, 220},
		"crimson":              {220, 20, 60},
		"cyan":                 {0, 255, 255},
		"darkblue":             {0, 0, 139},
		"darkcyan":             {0, 139, 139},
		"darkgoldenrod":        {184, 134, 11},
		"darkgray":             {169, 169, 169},
		"darkgreen":            {0, 100, 0},
		"darkgrey":             {169, 169, 169},
		"darkkhaki":            {189, 183, 107},
		"darkmagenta":          {139, 0, 139},
		"darkolivegreen":       {85, 107, 47},
		"darkorange":           {255, 140, 0},
		"darkorchid":           {153, 50, 204},
		"darkred":              {139, 0, 0},
		"darksalmon":           {233, 150, 122},
		"darkseagreen":         {143, 188, 143},
		"darkslateblue":        {72, 61, 139},
		"darkslategray":        {47, 79, 79},
		"darkslategrey":        {47, 79, 79},
		"darkturquoise":        {0, 206, 209},
		"darkviolet":           {148, 0, 211},
		"deeppink":             {255, 20, 147},
		"deepskyblue":          {0, 191, 255},
		"dimgray":              {105, 105, 105},
		"dimgrey":              {105, 105, 105},
		"dodgerblue":           {30, 144, 255},
		"firebrick":            {178, 34, 34},
		"floralwhite":          {255, 250, 240},
		"forestgreen":          {34, 139, 34},
		"fuchsia":              {255, 0, 255},
		"gainsboro":            {220, 220, 220},
		"ghostwhite":           {248, 248, 255},
		"gold":                 {255, 215, 0},
		"goldenrod":            {218, 165, 32},
		"gray":                 {128, 128, 128},
		"green":                {0, 128, 0},
		"greenyellow":          {173, 255, 47},
		"grey":                 {128, 128, 128},
		"honeydew":             {240, 255, 240},
		"hotpink":              {255, 105, 180},
		"indianred":            {205, 92, 92},
		"indigo":               {75, 0, 130},
		"ivory":                {255, 255, 240},
		"khaki":                {240, 230, 140},
		"lavender":             {230, 230, 250},
		"lavenderblush":        {255, 240, 245},
		"lawngreen":            {124, 252, 0},
		"lemonchiffon":         {255, 250, 205},
		"lightblue":            {173, 216, 230},
		"lightcoral":           {240, 128, 128},
		"lightcyan":            {224, 255, 255},
		"lightgoldenrodyellow": {250, 250, 210},
		"lightgray":            {211, 211, 211},
		"lightgreen":           {144, 238, 144},
		"lightgrey":            {211, 211, 211},
		"lightpink":            {255, 182, 193},
		"lightsalmon":          {255, 160, 122},
		"lightseagreen":        {32, 178, 170},
		"lightskyblue":         {135, 206, 250},
		"lightslategray":       {119, 136, 153},
		"lightslategrey":       {119, 136, 153},
		"lightsteelblue":       {176, 196, 222},
		"lightyellow":          {255, 255, 224},
		"lime":                 {0, 255, 0},
		"limegreen":            {50, 205, 50},
		"linen":                {250, 240, 230},
		"magenta":              {255, 0, 255},
		"maroon":               {128, 0, 0},
		"mediumaquamarine":     {102, 205, 170},
		"mediumblue":           {0, 0, 205},
		"mediumorchid":         {186, 85, 211},
		"mediumpurple":         {147, 112, 219},
		"mediumseagreen":       {60, 179, 113},
		"mediumslateblue":      {123, 104, 238},
		"mediumspringgreen":    {0, 250, 154},
		"mediumturquoise":      {72, 209, 204},
		"mediumvioletred":      {199, 21, 133},
		"midnightblue":         {25, 25, 112},
		"mintcream":            {245, 255, 250},
		"mistyrose":            {255, 228, 225},
		"moccasin":             {255, 228, 181},
		"navajowhite":          {255, 222, 173},
		"navy":                 {0, 0, 128},
		"oldlace":              {253, 245, 230},
		"olive":                {128, 128, 0},
		"olivedrab":            {107, 142, 35},
		"orange":               {255, 165, 0},
		"orangered":            {255, 69, 0},
		"orchid":               {218, 112, 214},
		"palegoldenrod":        {238, 232, 170},
		"palegreen":            {152, 251, 152},
		"paleturquoise":        {175, 238, 238},
		"palevioletred":        {219, 112, 147},
		"papayawhip":           {255, 239, 213},
		"peachpuff":            {255, 218, 185},
		"peru":                 {205, 133, 63},
		"pink":                 {255, 192, 203},
		"plum":                 {221, 160, 221},
		"powderblue":           {176, 224, 230},
		"purple":               {128, 0, 128},
		"rebeccapurple":        {102, 51, 153}, // CSS Color 4
		"red":                  {255, 0, 0},
		"rosybrown":            {188, 143, 143},
		"royalblue":            {65, 105, 225},
		"saddlebrown":          {139, 69, 19},
		"salmon":               {250, 128, 114},
		"sandybrown":           {244, 164, 96},
		"seagreen":             {46, 139, 87},
		"seashell":             {255, 245, 238},
		"sienna":               {160, 82, 45},
		"silver":               {192, 192, 192},
		"skyblue":              {135, 206, 235},
		"slateblue":            {106, 90, 205},
		"slategray":            {112, 128, 144},
		"slategrey":            {112, 128, 144},
		"snow":                 {255, 250, 250},
		"springgreen":          {0, 255, 127},
		"steelblue":            {70, 130, 180},
		"tan":                  {210, 180, 140},
		"teal":                 {0, 128, 128},
		"thistle":              {216, 191, 216},
		"tomato":               {255, 99, 71},
		"turquoise":            {64, 224, 208},
		"violet":               {238, 130, 238},
		"wheat":                {245, 222, 179},
		"white":                {255, 255, 255},
		"whitesmoke":           {245, 245, 245},
		"yellow":               {255, 255, 0},
		"yellowgreen":          {154, 205, 50},
	}
)

// parseColor4 parses the colors of CSS Color 4 (tinycss2 color4.parse_color),
// plus contrast-color().
func parseColor4(token Token, env colorEnv) (color.Value, ColorType) {
	switch token := token.(type) {
	case Ident:
		switch name := utils.AsciiLower(token.Value); name {
		case "currentcolor":
			if env.current != nil {
				return *env.current, ColorRGBA
			}
			return color.Value{}, ColorCurrentColor
		case "transparent":
			v := color.New(color.SRGB, 0, 0, 0)
			v.Alpha = 0
			return v, ColorRGBA
		default:
			if v, ok := colorKeywords[name]; ok {
				return v, ColorRGBA
			}
		}
	case Hash:
		if v, ok := parseHexColor(token.Value); ok {
			return v, ColorRGBA
		}
	case FunctionBlock:
		args := RemoveWhitespace(token.Arguments)
		name := utils.AsciiLower(token.Name)
		if name == "contrast-color" {
			return parseContrastColor(args, env)
		}
		var space Token
		if name == "color" {
			if len(args) == 0 {
				break
			}
			space, args = args[0], args[1:]
		}
		legacy := false
		switch {
		case (len(args) == 5 || len(args) == 7) && commasAtOddIndexes(args):
			legacy = true
			args = evenIndexes(args)
		case len(args) == 3:
		case len(args) == 5 && IsLiteral(args[3], "/"):
			args = []Token{args[0], args[1], args[2], args[4]}
		default:
			return color.Value{}, ColorInvalid
		}
		alpha, alphaMissing, ok := parseAlphaValue(args[3:], legacy)
		if !ok {
			break
		}
		channels := args[:3]
		if legacy && slices.ContainsFunc(channels, isNone) {
			break // the legacy syntax has no "none"
		}
		var v color.Value
		switch name {
		case "rgb", "rgba":
			v, ok = parseRGBChannels(channels, legacy)
		case "hsl", "hsla":
			v, ok = parseHSLChannels(channels, legacy)
		case "hwb":
			v, ok = parseHWBChannels(channels)
		case "lab":
			v, ok = parseLabChannels(color.Lab, channels, 1, 1.25, false)
		case "oklab":
			v, ok = parseLabChannels(color.OKLab, channels, 0.01, 0.004, false)
		case "lch":
			v, ok = parseLabChannels(color.LCH, channels, 1, 1.5, true)
		case "oklch":
			v, ok = parseLabChannels(color.OKLCH, channels, 0.01, 0.004, true)
		case "color":
			v, ok = parsePredefinedColor(space, channels)
		default:
			ok = false
		}
		// hwb(), lab(), lch(), oklab(), oklch() and color() have no legacy
		// syntax
		if legacy && name != "rgb" && name != "rgba" && name != "hsl" && name != "hsla" {
			ok = false
		}
		if ok {
			v.Alpha = alpha
			if alphaMissing {
				v.Missing |= 1 << color.AlphaMissing
			}
			return v, ColorRGBA
		}
	}
	return color.Value{}, ColorInvalid
}

// parseColor5 parses the functions added by CSS Color 5 (tinycss2
// color5.parse_color): device-cmyk(), color(--profile …) and light-dark().
func parseColor5(token Token, env colorEnv) (color.Value, ColorType) {
	fn, ok := token.(FunctionBlock)
	if !ok {
		return color.Value{}, ColorInvalid
	}
	args := RemoveWhitespace(fn.Arguments)
	name := utils.AsciiLower(fn.Name)
	var space Token
	if name == "color" {
		if len(args) == 0 {
			return color.Value{}, ColorInvalid
		}
		space, args = args[0], args[1:]
	}
	if name == "color-mix" {
		return parseColorMix(fn.Arguments, env)
	}
	if name == "light-dark" {
		// light-dark(<color>, <color>)
		if len(args) != 3 || !IsLiteral(args[1], ",") {
			return color.Value{}, ColorInvalid
		}
		light, lightType := parseColorEnv(args[0], env)
		dark, darkType := parseColorEnv(args[2], env)
		if lightType == ColorInvalid || darkType == ColorInvalid {
			return color.Value{}, ColorInvalid
		}
		if env.scheme == ColorSchemeDark {
			return dark, darkType
		}
		return light, lightType
	}
	var alphaArgs []Token
	// the legacy syntax has commas: a single argument is not legacy
	legacy := len(args) >= 3 && commasAtOddIndexes(args)
	if legacy {
		args = evenIndexes(args)
	} else {
		for i, arg := range args {
			if IsLiteral(arg, "/") {
				args, alphaArgs = args[:i], args[i+1:]
				if len(alphaArgs) == 0 { // "/" without alpha
					return color.Value{}, ColorInvalid
				}
				break
			}
		}
	}
	switch name {
	case "device-cmyk":
		alpha, alphaMissing, ok := parseAlphaValue(alphaArgs, legacy)
		if v, ok2 := parseCMYKChannels(args, legacy); ok && ok2 {
			v.Alpha = alpha
			if alphaMissing {
				v.Missing |= 1 << color.AlphaMissing
			}
			return v, ColorRGBA
		}
	case "color":
		// color(--profile <number>+ [/ alpha]): no legacy syntax
		if legacy || len(args) == 0 {
			return color.Value{}, ColorInvalid
		}
		alpha, alphaMissing, ok := parseAlphaValue(alphaArgs, false)
		if v, ok2 := parseProfileColor(space, args); ok && ok2 {
			v.Alpha = alpha
			if alphaMissing {
				v.Missing |= 1 << color.AlphaMissing
			}
			return v, ColorRGBA
		}
	}
	return color.Value{}, ColorInvalid
}

// parseContrastColor parses contrast-color(<color>) (CSS Color 5): white or
// black, whichever contrasts most with the color.
func parseContrastColor(args []Token, env colorEnv) (color.Value, ColorType) {
	if len(args) != 1 {
		return color.Value{}, ColorInvalid
	}
	bg, typ := parseColorEnv(args[0], env)
	if typ == ColorCurrentColor || typ == ColorDependsOnCurrent {
		return color.Value{}, ColorDependsOnCurrent
	}
	if typ != ColorRGBA {
		return color.Value{}, ColorInvalid
	}
	rgb, ok := bg.SRGB()
	if !ok {
		return color.Value{}, ColorInvalid
	}
	return color.Contrast(rgb), ColorRGBA
}

func commasAtOddIndexes(args []Token) bool {
	for i := 1; i < len(args); i += 2 {
		if !IsLiteral(args[i], ",") {
			return false
		}
	}
	return true
}

func evenIndexes(args []Token) []Token {
	out := make([]Token, 0, (len(args)+1)/2)
	for i := 0; i < len(args); i += 2 {
		out = append(out, args[i])
	}
	return out
}

// parseHexColor parses the 3, 4, 6 or 8 hexadecimal digits of a hash.
func parseHexColor(hex string) (color.Value, bool) {
	var digits int
	switch len(hex) {
	case 3, 4:
		digits = 1
	case 6, 8:
		digits = 2
	default:
		return color.Value{}, false
	}
	var channels [4]float64
	channels[3] = 255
	for i := 0; i*digits < len(hex); i++ {
		part := hex[i*digits : (i+1)*digits]
		if digits == 1 {
			part += part
		}
		n, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			return color.Value{}, false
		}
		channels[i] = float64(n)
	}
	v := color.New(color.SRGB, channels[0]/255, channels[1]/255, channels[2]/255)
	v.Alpha = channels[3] / 255
	return v, true
}

// numeric kinds of channel tokens; "none" is not a kind (tinycss2 _types
// ignores it).
type channelKinds struct {
	number, percentage, other bool
}

func kindsOf(args []Token) (k channelKinds) {
	for _, arg := range args {
		switch arg := arg.(type) {
		case Number:
			k.number = true
		case Percentage:
			k.percentage = true
		case Ident:
			if !isNone(arg) {
				k.other = true
			}
		default:
			k.other = true
		}
	}
	return k
}

func isNone(token Token) bool {
	ident, ok := token.(Ident)
	return ok && utils.AsciiLower(ident.Value) == "none"
}

// numericValue returns the value of a Number, Percentage or Dimension in
// float64 precision, read from the token's representation. Non-finite
// values are rejected.
func numericValue(token Token) (float64, bool) {
	var repr string
	var fallback float32
	switch token := token.(type) {
	case Number:
		repr, fallback = token.Value, token.ValueF
	case Percentage:
		repr, fallback = token.Value, token.ValueF
	case Dimension:
		repr, fallback = token.Value, token.ValueF
	default:
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(repr, "%"), 64)
	if err != nil {
		v = float64(fallback)
	}
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// channelValue reads a number, or a percentage scaled by percentScale, or
// "none". It does not check the token kinds, done by the callers.
func channelValue(token Token, numberScale, percentScale float64) (v float64, missing, ok bool) {
	if isNone(token) {
		return 0, true, true
	}
	v, ok = numericValue(token)
	if _, isPercentage := token.(Percentage); isPercentage {
		return v * percentScale, false, ok
	}
	return v * numberScale, false, ok
}

// parseHue reads a <hue>: a number (degrees), an angle, or "none".
// The result is in [0, 360).
func parseHue(token Token) (hue float64, missing, ok bool) {
	if isNone(token) {
		return 0, true, true
	}
	v, ok := numericValue(token)
	if !ok {
		return 0, false, false
	}
	switch token := token.(type) {
	case Number:
	case Dimension:
		switch utils.AsciiLower(token.Unit) {
		case "deg":
		case "grad":
			v = v / 400 * 360
		case "rad":
			v = v * 180 / math.Pi
		case "turn":
			v *= 360
		default:
			return 0, false, false
		}
	default:
		return 0, false, false
	}
	v = math.Mod(v, 360)
	if v < 0 {
		v += 360
	}
	return v, false, true
}

// parseAlphaValue reads an optional alpha: a number or a percentage,
// clamped to [0, 1], or "none" (missing, 0) outside the legacy syntax. No
// token means opaque.
func parseAlphaValue(args []Token, legacy bool) (alpha float64, missing, ok bool) {
	switch len(args) {
	case 0:
		return 1, false, true
	case 1:
		var v float64
		switch args[0].(type) {
		case Number:
			v, _ = numericValue(args[0])
		case Percentage:
			v, _ = numericValue(args[0])
			v /= 100
		default:
			if !legacy && isNone(args[0]) {
				return 0, true, true
			}
			return 0, false, false
		}
		return math.Max(0, math.Min(1, v)), false, true
	}
	return 0, false, false
}

// channelReader fills a Value from channel tokens, recording failures.
type channelReader struct {
	v  color.Value
	ok bool
}

func newChannelReader(space color.Space, count int) channelReader {
	return channelReader{v: color.Value{Space: space, Count: uint8(count), Alpha: 1}, ok: true}
}

func (r *channelReader) set(i int, v float64, missing, ok bool) {
	r.v.Components[i] = v
	if missing {
		r.v.Missing |= 1 << i
	}
	r.ok = r.ok && ok
}

func (r *channelReader) value(token Token, i int, numberScale, percentScale float64) {
	v, missing, ok := channelValue(token, numberScale, percentScale)
	r.set(i, v, missing, ok)
}

func (r *channelReader) hue(token Token, i int) {
	v, missing, ok := parseHue(token)
	r.set(i, v, missing, ok)
}

// rgb(): numbers in [0, 255] or percentages, mixed except in the legacy
// syntax, clamped to the range at parsed-value time (CSS Color 4 §5.1).
func parseRGBChannels(args []Token, legacy bool) (color.Value, bool) {
	if k := kindsOf(args); k.other || legacy && k.number && k.percentage {
		return color.Value{}, false
	}
	r := newChannelReader(color.SRGB, 3)
	for i, arg := range args {
		r.value(arg, i, 1.0/255, 0.01)
		r.v.Components[i] = math.Max(0, math.Min(1, r.v.Components[i]))
	}
	return r.v, r.ok
}

// hsl(): a hue, then saturation and lightness in [0, 100]: numbers or
// percentages, only percentages in the legacy syntax.
func parseHSLChannels(args []Token, legacy bool) (color.Value, bool) {
	if k := kindsOf(args[1:]); k.other || legacy && k.number {
		return color.Value{}, false
	}
	r := newChannelReader(color.HSL, 3)
	r.hue(args[0], 0)
	r.value(args[1], 1, 1, 1)
	r.value(args[2], 2, 1, 1)
	// CSS Color 4 §7: a negative saturation is clamped to 0 at
	// parsed-value time
	r.v.Components[1] = math.Max(0, r.v.Components[1])
	return r.v, r.ok
}

// hwb(): a hue, then whiteness and blackness in [0, 100].
func parseHWBChannels(args []Token) (color.Value, bool) {
	if kindsOf(args[1:]).other {
		return color.Value{}, false
	}
	r := newChannelReader(color.HWB, 3)
	r.hue(args[0], 0)
	r.value(args[1], 1, 1, 1)
	r.value(args[2], 2, 1, 1)
	return r.v, r.ok
}

// lab(), oklab(), lch(), oklch(): lightness (100% maps to 100 for lab/lch,
// 1 for the ok spaces), then a/b or chroma (percentages scaled by
// percentScale), then for the polar spaces a hue.
//
// As CSS Color 4 §8 and §9 require, lightness is clamped to [0%, 100%] and
// chroma to positive values at parsed-value time. (tinycss2, and so the
// css-parsing-tests corpus, keeps them unclamped.)
func parseLabChannels(space color.Space, args []Token, lightnessPercent, percentScale float64, polar bool) (color.Value, bool) {
	checked := args
	if polar {
		checked = args[:2]
	}
	if kindsOf(checked).other {
		return color.Value{}, false
	}
	r := newChannelReader(space, 3)
	r.value(args[0], 0, 1, lightnessPercent)
	r.value(args[1], 1, 1, percentScale)
	if polar {
		r.hue(args[2], 2)
		r.v.Components[1] = math.Max(0, r.v.Components[1])
	} else {
		r.value(args[2], 2, 1, percentScale)
	}
	r.v.Components[0] = math.Max(0, math.Min(100*lightnessPercent, r.v.Components[0]))
	return r.v, r.ok
}

// color(<predefined space> c1 c2 c3): numbers, or percentages of 1.
func parsePredefinedColor(space Token, args []Token) (color.Value, bool) {
	ident, ok := space.(Ident)
	if !ok || kindsOf(args).other {
		return color.Value{}, false
	}
	s, ok := color.PredefinedSpace(utils.AsciiLower(ident.Value))
	if !ok {
		return color.Value{}, false
	}
	r := newChannelReader(s, 3)
	for i, arg := range args {
		r.value(arg, i, 1, 0.01)
	}
	return r.v, r.ok
}

// color(--profile c1 … cn): a color in a space defined by @color-profile.
func parseProfileColor(space Token, args []Token) (color.Value, bool) {
	ident, ok := space.(Ident)
	if !ok || !strings.HasPrefix(ident.Value, "--") || kindsOf(args).other ||
		len(args) > color.MaxComponents {
		return color.Value{}, false
	}
	r := newChannelReader(color.Custom, len(args))
	r.v.Profile = ident.Value
	for i, arg := range args {
		r.value(arg, i, 1, 0.01)
	}
	return r.v, r.ok
}

// device-cmyk(c m y k): numbers (only numbers in the legacy comma syntax)
// or percentages, clamped to [0, 1].
func parseCMYKChannels(args []Token, legacy bool) (color.Value, bool) {
	k := kindsOf(args)
	// the legacy syntax is device-cmyk(<number>#{4}): no percentage, no none
	if len(args) != 4 || k.other || legacy && (k.percentage || slices.ContainsFunc(args, isNone)) {
		return color.Value{}, false
	}
	r := newChannelReader(color.DeviceCMYK, 4)
	for i, arg := range args {
		r.value(arg, i, 1, 0.01)
		r.v.Components[i] = math.Max(0, math.Min(1, r.v.Components[i]))
	}
	return r.v, r.ok
}

// parseColorMix parses color-mix() (CSS Color 5 §2):
//
//	color-mix(in <colorspace> [<hue-interpolation-method> hue]?, [<color> && <percentage>?]#{2})
//
// A mix of currentcolor is [ColorDependsOnCurrent] when its value is not
// known: it is mixed again once it is (see [Color.Resolve]).
func parseColorMix(arguments []Token, env colorEnv) (color.Value, ColorType) {
	parts := SplitOnComma(RemoveWhitespace(arguments))
	if len(parts) != 3 {
		return color.Value{}, ColorInvalid
	}
	// the interpolation method
	method := parts[0]
	if len(method) < 2 {
		return color.Value{}, ColorInvalid
	}
	if in, _ := method[0].(Ident); utils.AsciiLower(in.Value) != "in" {
		return color.Value{}, ColorInvalid
	}
	spaceName, ok := method[1].(Ident)
	if !ok {
		return color.Value{}, ColorInvalid
	}
	space, ok := mixSpace(utils.AsciiLower(spaceName.Value))
	if !ok {
		return color.Value{}, ColorInvalid
	}
	hue := color.HueShorter
	switch len(method) {
	case 2:
	case 4:
		if !space.IsPolar() {
			return color.Value{}, ColorInvalid
		}
		m, _ := method[2].(Ident)
		h, _ := method[3].(Ident)
		if utils.AsciiLower(h.Value) != "hue" {
			return color.Value{}, ColorInvalid
		}
		switch utils.AsciiLower(m.Value) {
		case "shorter":
		case "longer":
			hue = color.HueLonger
		case "increasing":
			hue = color.HueIncreasing
		case "decreasing":
			hue = color.HueDecreasing
		default:
			return color.Value{}, ColorInvalid
		}
	default:
		return color.Value{}, ColorInvalid
	}

	// the two colors, each with an optional percentage before or after it
	var (
		colors    [2]color.Value
		pcts      [2]float64
		hasPct    [2]bool
		dependent bool
	)
	for i, part := range parts[1:] {
		var colorToken Token
		for _, t := range part {
			if p, isPct := t.(Percentage); isPct && !hasPct[i] {
				v, _ := numericValue(p)
				if v < 0 || v > 100 {
					return color.Value{}, ColorInvalid
				}
				pcts[i], hasPct[i] = v, true
			} else if colorToken == nil {
				colorToken = t
			} else {
				return color.Value{}, ColorInvalid
			}
		}
		if colorToken == nil {
			return color.Value{}, ColorInvalid
		}
		v, typ := parseColorEnv(colorToken, env)
		switch typ {
		case ColorRGBA:
			// checked now even when the mix waits for currentcolor
			if _, ok := v.To(space); !ok {
				return color.Value{}, ColorInvalid
			}
			colors[i] = v
		case ColorCurrentColor, ColorDependsOnCurrent:
			dependent = true
		default:
			return color.Value{}, ColorInvalid
		}
	}

	// percentage normalization (§2.1)
	switch {
	case !hasPct[0] && !hasPct[1]:
		pcts = [2]float64{50, 50}
	case !hasPct[1]:
		pcts[1] = 100 - pcts[0]
	case !hasPct[0]:
		pcts[0] = 100 - pcts[1]
	}
	sum := pcts[0] + pcts[1]
	if sum == 0 {
		return color.Value{}, ColorInvalid
	}
	if dependent { // the syntax is valid: mixed once currentcolor is known
		return color.Value{}, ColorDependsOnCurrent
	}
	alphaMultiplier := 1.0
	if sum < 100 {
		alphaMultiplier = sum / 100
	}
	out, ok := color.Mix(space, hue, colors[0], colors[1], pcts[1]/sum, alphaMultiplier)
	if !ok {
		return color.Value{}, ColorInvalid
	}
	return out, ColorRGBA
}

// mixSpace returns the <colorspace> of color-mix() named name.
func mixSpace(name string) (color.Space, bool) {
	switch name {
	case "hsl":
		return color.HSL, true
	case "hwb":
		return color.HWB, true
	case "lab":
		return color.Lab, true
	case "lch":
		return color.LCH, true
	case "oklab":
		return color.OKLab, true
	case "oklch":
		return color.OKLCH, true
	}
	return color.PredefinedSpace(name)
}
