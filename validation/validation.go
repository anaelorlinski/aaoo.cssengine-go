package validation

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/logger"

	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
	"github.com/anaelorlinski/aaoo.cssengine-go/selector"
)

// Expand shorthands and validate property values.
// See http://www.w3.org/TR/CSS21/propidx.html and various CSS3 modules.

// :copyright: Copyright 2011-2014 Simon Sapin and contributors, see AUTHORS.
// :license: BSD, see LICENSE for details.
// (WeasyPrint's header: AUTHORS is in its repository,
// https://github.com/Kozea/WeasyPrint; the license is validation/LICENSE.)

const proprietaryPrefix = "-weasy-"

// LengthUnit returns the length unit named name (lowercase: "px", "em",
// "vw"…), or false.
func LengthUnit(name string) (pr.Unit, bool) {
	u, ok := lengthUnits[name]
	return u, ok
}

// AngleUnit returns the angle unit named name (lowercase: "deg", "rad",
// "grad", "turn"), or false.
func AngleUnit(name string) (pr.Unit, bool) {
	u, ok := angleUnits[name]
	return u, ok
}

// buildLengthUnits returns the unit-name → enum map used by [getLength].
//
// The viewport families are generated rather than spelled out: in print
// there is no interface chrome, so the small/large/dynamic variants
// (sv*, lv*, dv*) are by definition equal to the unprefixed v* ones and
// are aliased onto the same enum values. Only pv* — page-box relative,
// as opposed to page-area relative — is genuinely distinct.
func buildLengthUnits() map[string]pr.Unit {
	out := map[string]pr.Unit{
		"ex": pr.Ex, "em": pr.Em, "ch": pr.Ch, "rem": pr.Rem, "lh": pr.Lh, "rlh": pr.Rlh,
		"px": pr.Px, "pt": pr.Pt, "pc": pr.Pc, "in": pr.In, "cm": pr.Cm, "mm": pr.Mm, "q": pr.Q,
	}
	axes := map[string][2]pr.Unit{
		// suffix -> {page area unit, page box unit}
		"w": {pr.Vw, pr.Pvw}, "h": {pr.Vh, pr.Pvh},
		"i": {pr.Vi, pr.Pvi}, "b": {pr.Vb, pr.Pvb},
		"min": {pr.Vmin, pr.Pvmin}, "max": {pr.Vmax, pr.Pvmax},
	}
	for suffix, units := range axes {
		for _, prefix := range [...]string{"", "s", "l", "d"} {
			out[prefix+"v"+suffix] = units[0]
		}
		out["pv"+suffix] = units[1]
	}
	return out
}

var (
	errInvalidValue = errors.New("invalid or unsupported values for a known CSS property")

	lengthUnits = buildLengthUnits()
	angleUnits  = map[string]pr.Unit{"rad": pr.Rad, "turn": pr.Turn, "deg": pr.Deg, "grad": pr.Grad}
	// keyword -> (open, insert)
	contentQuoteKeywords = map[string]pr.Quote{
		"open-quote":     {Open: true, Insert: true},
		"close-quote":    {Open: false, Insert: true},
		"no-open-quote":  {Open: true, Insert: false},
		"no-close-quote": {Open: false, Insert: false},
	}

	zeroPercent    = pr.PercToD(0)
	fiftyPercent   = pr.PercToD(50)
	hundredPercent = pr.PercToD(100)

	backgroundPositionsPercentages = map[string]pr.Dimension{
		"top":    zeroPercent,
		"left":   zeroPercent,
		"center": fiftyPercent,
		"bottom": hundredPercent,
		"right":  hundredPercent,
	}

	// http://drafts.csswg.org/csswg/css3-values/#angles
	// 1<unit> is this many radians.
	angleToRadians = map[pr.Unit]float32{
		pr.Rad:  1,
		pr.Turn: 2 * math.Pi,
		pr.Deg:  math.Pi / 180,
		pr.Grad: math.Pi / 200,
	}

	// http://drafts.csswg.org/csswg/css-values/#resolution
	// Per WP commit c57a44ef: "x" is an alias for "dppx".
	resolutionToDppx = map[string]float32{
		"dppx": 1,
		"x":    1,
		"dpi":  float32(1 / pr.LengthToPixels(pr.In)),
		"dpcm": float32(1 / pr.LengthToPixels(pr.Cm)),
	}

	couplesLigatures = [][]string{
		{"common-ligatures", "no-common-ligatures"},
		{"historical-ligatures", "no-historical-ligatures"},
		{"discretionary-ligatures", "no-discretionary-ligatures"},
		{"contextual", "no-contextual"},
	}
	couplesNumeric = [][]string{
		{"lining-nums", "oldstyle-nums"},
		{"proportional-nums", "tabular-nums"},
		{"diagonal-fractions", "stacked-fractions"},
		{"ordinal"},
		{"slashed-zero"},
	}

	couplesEastAsian = [][]string{
		{"jis78", "jis83", "jis90", "jis04", "simplified", "traditional"},
		{"full-width", "proportional-width"},
		{"ruby"},
	}

	allLigaturesValues = utils.Set{}
	allNumericValues   = utils.Set{}
	allEastAsianValues = utils.Set{}

	// yes/no validators for non-shorthand properties
	// Maps property names to functions taking a property name and a value list,
	// returning a value or None for invalid.
	// Also transform values: keyword and URLs are returned as strings.
	// For properties that take a single value, that value is returned by itself
	// instead of a list.
	// Flat array for the generated range, with registeredValidators beside
	// it for setup-registered properties — same reasoning as
	// html/tree.computerFunctions: a map for everything costs every
	// built-in lookup ~4.8x.
	validators = [pr.NbProperties]validator{
		pr.PAppearance:              appearance,
		pr.PBackgroundAttachment:    backgroundAttachment,
		pr.PBackgroundColor:         otherColors,
		pr.PBorderTopColor:          otherColors,
		pr.PBorderRightColor:        otherColors,
		pr.PBorderBottomColor:       otherColors,
		pr.PBorderLeftColor:         otherColors,
		pr.PColumnRuleColor:         otherColors,
		pr.PTextDecorationColor:     otherColors,
		pr.POutlineColor:            outlineColor,
		pr.PBorderCollapse:          borderCollapse,
		pr.PEmptyCells:              emptyCells,
		pr.PTransformOrigin:         transformOrigin,
		pr.PObjectPosition:          objectPosition,
		pr.PBackgroundPosition:      backgroundPosition,
		pr.PBackgroundRepeat:        backgroundRepeat,
		pr.PBackgroundSize:          backgroundSize,
		pr.PBackgroundClip:          box,
		pr.PBackgroundOrigin:        box,
		pr.PBorderSpacing:           borderSpacing,
		pr.PBorderTopRightRadius:    borderCornerRadius,
		pr.PBorderBottomRightRadius: borderCornerRadius,
		pr.PBorderBottomLeftRadius:  borderCornerRadius,
		pr.PBorderTopLeftRadius:     borderCornerRadius,
		pr.PBorderTopStyle:          borderStyle,
		pr.PBorderRightStyle:        borderStyle,
		pr.PBorderLeftStyle:         borderStyle,
		pr.PBorderBottomStyle:       borderStyle,
		pr.PColumnRuleStyle:         borderStyle,
		pr.PBreakBefore:             breakBeforeAfter,
		pr.PBreakAfter:              breakBeforeAfter,
		pr.PBreakInside:             breakInside,
		pr.PBoxDecorationBreak:      boxDecorationBreak,
		pr.PMarginBreak:             marginBreak,
		pr.PPage:                    page,
		pr.PBleedLeft:               bleed,
		pr.PBleedRight:              bleed,
		pr.PBleedTop:                bleed,
		pr.PBleedBottom:             bleed,
		pr.PMarks:                   marks,
		pr.POutlineStyle:            outlineStyle,
		pr.PBorderTopWidth:          borderWidth,
		pr.PBorderRightWidth:        borderWidth,
		pr.PBorderLeftWidth:         borderWidth,
		pr.PBorderBottomWidth:       borderWidth,
		pr.PBorderImageSlice:        borderImageSlice,
		pr.PBorderImageWidth:        borderImageWidth,
		pr.PBorderImageOutset:       borderImageOutset,
		pr.PBorderImageRepeat:       borderImageRepeat,
		pr.PMaskBorderSlice:         borderImageSlice,
		pr.PMaskBorderWidth:         borderImageWidth,
		pr.PMaskBorderOutset:        borderImageOutset,
		pr.PMaskBorderRepeat:        borderImageRepeat,
		pr.PMaskBorderMode:          maskBorderMode,
		pr.PColumnRuleWidth:         borderWidth,
		pr.POutlineWidth:            borderWidth,
		pr.PColumnWidth:             columnWidth,
		pr.PColumnSpan:              columnSpan,
		pr.PBoxSizing:               boxSizing,
		pr.PCaptionSide:             captionSide,
		pr.PClear:                   clear,
		pr.PClip:                    clip,
		pr.PTop:                     lengthOrAuto,
		pr.PRight:                   lengthOrAuto,
		pr.PLeft:                    lengthOrAuto,
		pr.PBottom:                  lengthOrAuto,
		pr.PMarginTop:               lengthOrAuto,
		pr.PMarginRight:             lengthOrAuto,
		pr.PMarginBottom:            lengthOrAuto,
		pr.PMarginLeft:              lengthOrAuto,
		pr.PHeight:                  widthHeight,
		pr.PWidth:                   widthHeight,
		pr.PColumnFill:              columnFill,
		pr.PDirection:               direction,
		pr.PDisplay:                 display,
		pr.PFloat:                   float,
		pr.PFontFamily:              fontFamily,
		pr.PFontKerning:             fontKerning,
		pr.PFontLanguageOverride:    fontLanguageOverride,
		pr.PFontVariantLigatures:    fontVariantLigatures,
		pr.PFontVariantPosition:     fontVariantPosition,
		pr.PFontVariantCaps:         fontVariantCaps,
		pr.PFontVariantNumeric:      fontVariantNumeric,
		pr.PFontFeatureSettings:     fontFeatureSettings,
		pr.PFontVariantAlternates:   fontVariantAlternates,
		pr.PFontVariantEastAsian:    fontVariantEastAsian,
		pr.PFontVariationSettings:   fontVariationSettings,
		pr.PFontStyle:               fontStyle,
		pr.PFontStretch:             fontStretch,
		pr.PFontWeight:              fontWeight,
		pr.PFootnoteDisplay:         footnoteDisplay,
		pr.PFootnotePolicy:          footnotePolicy,
		pr.PImageResolution:         imageResolution,
		pr.PLetterSpacing:           spacing,
		pr.PWordSpacing:             spacing,
		pr.PLineHeight:              lineHeight,
		pr.PListStylePosition:       listStylePosition,
		pr.PListStyleType:           listStyleType,
		pr.PPaddingTop:              lengthOrPercentage,
		pr.PPaddingRight:            lengthOrPercentage,
		pr.PPaddingBottom:           lengthOrPercentage,
		pr.PPaddingLeft:             lengthOrPercentage,
		pr.PMinWidth:                minWidthHeight,
		pr.PMinHeight:               minWidthHeight,
		pr.PMaxWidth:                maxWidthHeight,
		pr.PMaxHeight:               maxWidthHeight,
		pr.POpacity:                 opacity,
		pr.PZIndex:                  zIndex,
		pr.POrphans:                 orphansWidows,
		pr.PWidows:                  orphansWidows,
		pr.PColumnCount:             columnCount,
		pr.POverflow:                overflow,
		pr.PPosition:                position,
		pr.PQuotes:                  quotes,
		pr.PTableLayout:             tableLayout,
		pr.PTextAlignAll:            textAlignAll,
		pr.PTextAlignLast:           textAlignLast,
		pr.PTextDecorationLine:      textDecorationLine,
		pr.PTextDecorationStyle:     textDecorationStyle,
		pr.PTextDecorationThickness: textDecorationThickness,
		pr.PTextUnderlineOffset:     textUnderlineOffset,
		pr.PTextIndent:              textIndent,
		pr.PTextTransform:           textTransform,
		pr.PVerticalAlign:           verticalAlign,
		pr.PVisibility:              visibility,
		pr.PWhiteSpace:              whiteSpace,
		pr.POverflowWrap:            overflowWrap,
		pr.PImageRendering:          imageRendering,
		pr.PImageOrientation:        imageOrientation,
		pr.PSize:                    size,
		pr.PTabSize:                 tabSize,
		pr.PHyphens:                 hyphens,
		pr.PHyphenateCharacter:      hyphenateCharacter,
		pr.PHyphenateLimitZone:      hyphenateLimitZone,
		pr.PHyphenateLimitChars:     hyphenateLimitChars,
		pr.PLang:                    lang,
		pr.PBookmarkLevel:           bookmarkLevel,
		pr.PBookmarkState:           bookmarkState,
		pr.PObjectFit:               objectFit,
		pr.PTextOverflow:            textOverflow,
		pr.PFlexBasis:               flexBasis,
		pr.PFlexDirection:           flexDirection,
		pr.PFlexGrow:                flexGrowShrink,
		pr.PFlexShrink:              flexGrowShrink,
		pr.POrder:                   order,
		pr.PColumnGap:               gap,
		pr.PRowGap:                  gap,
		pr.PFlexWrap:                flexWrap,
		pr.PAlignContent:            alignContent,
		pr.PAlignItems:              alignItems,
		pr.PAlignSelf:               alignSelf,
		pr.PJustifyContent:          justifyContent,
		pr.PJustifyItems:            justifyItems,
		pr.PJustifySelf:             justifySelf,
		pr.PAnchor:                  anchor,
		pr.PBlockEllipsis:           blockEllipsis,
		pr.PContinue:                continue_,
		pr.PMaxLines:                maxLines,
		pr.PWordBreak:               wordBreak,
		pr.PGridAutoColumns:         gridAuto,
		pr.PGridAutoRows:            gridAuto,
		pr.PGridAutoFlow:            gridAutoFlow,
		pr.PGridTemplateColumns:     gridTemplate,
		pr.PGridTemplateRows:        gridTemplate,
		pr.PGridTemplateAreas:       gridTemplateAreas,
		pr.PGridRowStart:            gridLine,
		pr.PGridColumnStart:         gridLine,
		pr.PGridRowEnd:              gridLine,
		pr.PGridColumnEnd:           gridLine,
		pr.PBoxShadow:               boxShadow,
		pr.PTextShadow:              textShadow,
	}
	validatorsError = map[pr.KnownProp]validatorError{
		pr.PBackgroundImage:   backgroundImage,
		pr.PBorderImageSource: borderImageSource,
		pr.PMaskBorderSource:  borderImageSource,
		pr.PListStyleImage:    listStyleImage,
		pr.PContent:           content,
		pr.PCounterIncrement:  counterIncrement,
		pr.PCounterReset:      counterReset,
		pr.PCounterSet:        counterReset,
		pr.PFontSize:          fontSize,
		pr.PBookmarkLabel:     bookmarkLabel,
		pr.PTransform:         transform,
		pr.PStringSet:         stringSet,
		pr.PLink:              link,
	}

	// regroup the two cases (with error or without error)
	allValidators = pr.NewSetK(pr.PColor) // special case because of inherited

	proprietary = utils.NewSet(
		"anchor",
		"link",
		"lang",
	)
	unstable = utils.NewSet(
		"transform-origin",
		"size",
		"hyphens",
		"hyphenate-character",
		"hyphenate-limit-zone",
		"hyphenate-limit-chars",
		"bookmark-label",
		"bookmark-level",
		"bookmark-state",
		"string-set",
		"column-rule-color",
		"column-rule-style",
		"column-rule-width",
		"column-width",
		"column-span",
		"column-gap",
		"column-fill",
		"column-count",
		"bleed-left",
		"bleed-right",
		"bleed-top",
		"bleed-bottom",
		"marks",
		"continue",
		"max-lines",
	)
)

func init() {
	for _, couple := range couplesLigatures {
		for _, cc := range couple {
			allLigaturesValues[cc] = utils.Has
		}
	}
	for _, couple := range couplesNumeric {
		for _, cc := range couple {
			allNumericValues[cc] = utils.Has
		}
	}
	for _, couple := range couplesEastAsian {
		for _, cc := range couple {
			allEastAsianValues[cc] = utils.Has
		}
	}
	for name, v := range validators {
		if v != nil {
			allValidators.Add(pr.KnownProp(name))
		}
	}
	for name := range validatorsError {
		allValidators.Add(name)
	}
}

type Token = pa.Token

type (
	validator      func(tokens []Token, baseURL string) pr.CssProperty // dont support var(), attr()
	validatorError func(tokens []Token, baseURL string) (pr.CssProperty, error)
)

// registeredValidators holds the validators of properties registered at
// setup time, which cannot fit the fixed array.
var registeredValidators = map[pr.KnownProp]validator{}

// RegisterValidator installs the validator of a property registered through
// [properties.RegisterProperty] (it panics for another one). Setup-time
// only: like RegisterProperty, it panics once the registry is sealed (the
// first stylesheet parsed), as the validators are then read without
// synchronisation.
func RegisterValidator(p pr.KnownProp, v func(tokens []Token, baseURL string) pr.CssProperty) {
	if !p.IsRegistered() { // the validators of built-in properties are fixed
		panic(fmt.Sprintf("validation: RegisterValidator(%v): not a property registered by RegisterProperty", p))
	}
	pr.Registering("RegisterValidator", func() {
		registeredValidators[p] = v
		allValidators.Add(p)
	})
}

// validatorFor returns the validator of p, or nil.
func validatorFor(p pr.KnownProp) validator {
	if int(p) < len(validators) {
		return validators[p]
	}
	return registeredValidators[p]
}

// ValidateKnown validate one known, non shorthand, property.
func ValidateKnown(name pr.KnownProp, tokens []Token, baseURL string) (out pr.DeclaredValue, err error) {
	if name == pr.PColor { // special case to handle inherit
		return color(tokens, ""), nil
	}

	var value pr.CssProperty
	if function := validatorFor(name); function != nil {
		value = function(tokens, baseURL)
	} else if functionE := validatorsError[name]; functionE != nil {
		value, err = functionE(tokens, baseURL)
	}
	return value, err
}

// Validate validates the value tokens of one property, a longhand or a
// custom property, as [PreprocessDeclarations] does for a declaration: it
// returns the typed value, a CSS-wide keyword, raw tokens for a value
// holding var(), or an error for an invalid value. It is used after var()
// substitution.
func Validate(key pr.PropKey, tokens []Token) (pr.DeclaredValue, error) {
	out, err := validateNonShorthand("", key.String(), tokens, false)
	return out.property, err
}

// Default validator for non-shorthand pr.
// required = false
func validateNonShorthand(baseURL string, name string, tokens []pa.Token, required bool) (out namedProperty, err error) {
	if strings.HasPrefix(name, "--") { // variable
		// can't validate variables contents before substitution
		return namedProperty{
			name:     pr.PropKey{Var: name},
			property: pr.RawTokens(tokens),
		}, nil
	}

	prop := pr.PropertyByName(name)
	if !required && !pr.IsKnownProperty(prop) {
		return out, errors.New("unknown property")
	}

	if _, isSupported := allValidators[prop]; !required && !isSupported {
		return out, fmt.Errorf("property %s not supported yet", name)
	}

	for _, token := range tokens {
		if HasVar(token) {
			// Found CSS variable, return pending-substitution values.
			return namedProperty{name: pr.PropKey{KnownProp: prop}, property: pr.RawTokens(tokens)}, nil
		}
	}

	var value pr.DeclaredValue
	if v, ok := cssWideKeyword(getSingleKeyword(tokens), prop); ok {
		value = v
	} else {
		value, err = ValidateKnown(prop, tokens, baseURL)
		if err != nil {
			return out, err
		}
		if value == nil {
			return out, errors.New("invalid value (nil function return)")
		}
	}

	return namedProperty{name: pr.PropKey{KnownProp: prop}, property: value}, nil
}

// Not applicable to the print media
var notPrintMedia = utils.NewSet(
	// Aural media
	"azimuth",
	"cue",
	"cue-after",
	"cue-before",
	"elevation",
	"pause",
	"pause-after",
	"pause-before",
	"pitch-range",
	"pitch",
	"play-during",
	"richness",
	"speak-header",
	"speak-numeral",
	"speak-punctuation",
	"speak",
	"speech-rate",
	"stress",
	"voice-family",
	"volume",
	// Animations, transitions, timelines
	"animation",
	"animation-composition",
	"animation-delay",
	"animation-direction",
	"animation-duration",
	"animation-fill-mode",
	"animation-iteration-count",
	"animation-name",
	"animation-play-state",
	"animation-range",
	"animation-range-end",
	"animation-range-start",
	"animation-timeline",
	"animation-timing-function",
	"timeline-scope",
	"transition",
	"transition-delay",
	"transition-duration",
	"transition-property",
	"transition-timing-function",
	"view-timeline",
	"view-timeline-axis",
	"view-timeline-inset",
	"view-timeline-name",
	"view-transition-name",
	"will-change",
	// Dynamic and interactive
	"caret",
	"caret-color",
	"caret-shape",
	"cursor",
	"field-sizing",
	"pointer-events",
	"resize",
	"touch-action",
	// Browser viewport scrolling
	"overscroll-behavior",
	"overscroll-behavior-block",
	"overscroll-behavior-inline",
	"overscroll-behavior-x",
	"overscroll-behavior-y",
	"scroll-behavior",
	"scroll-margin",
	"scroll-margin-block",
	"scroll-margin-block-end",
	"scroll-margin-block-start",
	"scroll-margin-bottom",
	"scroll-margin-inline",
	"scroll-margin-inline-end",
	"scroll-margin-inline-start",
	"scroll-margin-left",
	"scroll-margin-right",
	"scroll-margin-top",
	"scroll-padding",
	"scroll-padding-block",
	"scroll-padding-block-end",
	"scroll-padding-block-start",
	"scroll-padding-bottom",
	"scroll-padding-inline",
	"scroll-padding-inline-end",
	"scroll-padding-inline-start",
	"scroll-padding-left",
	"scroll-padding-right",
	"scroll-padding-top",
	"scroll-snap-align",
	"scroll-snap-stop",
	"scroll-snap-type",
	"scroll-timeline",
	"scroll-timeline-axis",
	"scroll-timeline-name",
	"scrollbar-color",
	"scrollbar-gutter",
	"scrollbar-width",
)

// Declaration is the input form of a CSS property,
// possibly containing variables.
type Declaration struct {
	Name  pr.PropKey
	Value pr.DeclaredValue

	// Shorthand is not zero for shorthands containing 'var()' tokens, waiting to be expanded and validated
	// In this case, [Value] is [pr.RawTokens] and refers to the associate shorthand, not to the expanded [Name]
	Shorthand pr.Shorthand

	Important bool
}

// KeyedDeclarations are the declarations of a style rule, with its
// selector.
type KeyedDeclarations struct {
	Selector     selector.SelectorGroup
	Declarations []Declaration
}

var (
	pos11 = pa.Pos{Line: 1, Column: 1}
	colon = pa.NewLiteral(":", pos11)
)

// PreprocessDeclarations validates declarations: see
// [PreprocessDeclarationsPrelude].
func PreprocessDeclarations(baseURL string, declarations []pa.Compound) []Declaration {
	tmp, _ := PreprocessDeclarationsPrelude(baseURL, declarations, nil)
	return tmp[0].Declarations
}

// PreprocessDeclarationsPrelude filter unsupported properties or parsing errors,
// and expand shorthand properties.
//
// Properties containing var() tokens are not validated yet.
// Shorthand containing var() tokens are not expanded.
//
// Log a warning for every ignored declaration.
//
// If [prelude] is nil, the returned error is always nil.
// The returned slice is never empty, and has always length 1 if [prelude] is nil.
func PreprocessDeclarationsPrelude(baseURL string, declarations []pa.Compound, prelude []pa.Token) ([]KeyedDeclarations, error) {
	// Parsing starts: property registration (setup time) is over.
	pr.Seal()

	// Compile list of selectors.
	var selectors selector.SelectorGroup
	if prelude != nil {
		// Handle & selector in non-nested rule. MDN explains that & is
		// then equivalent to :scope, and :scope is equivalent to :root
		// as we don’t support :scope yet.
		originalPrelude := prelude
		prelude = []Token{}
		for _, token := range originalPrelude {
			if pa.IsLiteral(token, "&") {
				prelude = append(prelude, colon, pa.NewIdent("root", pos11))
			} else {
				prelude = append(prelude, token)
			}
		}
		var err error
		selectors, err = selector.ParseGroup(pa.Serialize(prelude))
		if err != nil {
			return nil, err
		}
	}

	// Yield declarations.
	is := pa.NewFunctionBlock(pos11, "is", prelude)
	var (
		out      []KeyedDeclarations
		ownDecls []Declaration
	)
	for _, declaration := range declarations {
		if errToken, ok := declaration.(pa.ParseError); ok {
			logger.L().Warn("Parse error", "reason", errToken.Message)
		}

		if declaration, ok := declaration.(pa.QualifiedRule); ok {
			// Nested rule.
			if prelude == nil {
				continue
			}
			hasNesting := false
			// Replace & selector by parent.
			var declarationPrelude []Token
			for _, token := range declaration.Prelude {
				if pa.IsLiteral(token, "&") {
					hasNesting = true
					declarationPrelude = append(declarationPrelude, colon, is)
				} else {
					declarationPrelude = append(declarationPrelude, token)
				}
			}
			if !hasNesting {
				// No & selector, prepend parent.
				declarationPrelude = append([]Token{colon, is, pa.NewWhitespace(" ", pos11)},
					declaration.Prelude...)
			}
			contents, err := PreprocessDeclarationsPrelude(baseURL, pa.ParseBlocksContents(declaration.Content, false),
				declarationPrelude)
			if err != nil {
				return nil, err
			}
			out = append(out, contents...)
		}

		declaration, ok := declaration.(pa.Declaration)
		if !ok {
			continue
		}

		name := declaration.Name
		if !strings.HasPrefix(name, "--") { // check for non variable, case insensitive
			name = utils.AsciiLower(declaration.Name)
		}

		validationError := func(reason string) {
			logger.L().Warn("Ignored declaration", "name", declaration.Name, "value", strings.TrimSpace(pa.Serialize(declaration.Value)), "reason", reason)
		}

		if _, in := notPrintMedia[name]; in {
			validationError("the property does not apply for the print media")
			continue
		}

		if strings.HasPrefix(name, proprietaryPrefix) {
			unprefixedName := strings.TrimPrefix(name, proprietaryPrefix)
			if _, in := proprietary[unprefixedName]; in {
				name = unprefixedName
			} else if _, in := unstable[unprefixedName]; in {
				logger.L().Warn("Deprecated declaration: prefixes on unstable attributes are deprecated", "name", declaration.Name,
					"value", strings.TrimSpace(pa.Serialize(declaration.Value)), "use", unprefixedName)
				name = unprefixedName
			} else {
				logger.L().Warn("Ignored declaration: prefix on this attribute is not supported", "name", declaration.Name,
					"value", strings.TrimSpace(pa.Serialize(declaration.Value)), "use", unprefixedName)
				continue
			}
		}

		if strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "--") {
			validationError("prefixed selectors are ignored")
			continue
		}

		tokens := pa.RemoveWhitespace(declaration.Value)
		if strings.HasPrefix(name, "--") || slices.ContainsFunc(tokens, HasVar) {
			// validated after var() substitution, without the sheet: its
			// relative urls are resolved now, against the sheet declaring
			// them
			tokens = absoluteURLs(tokens, baseURL)
		}

		// Having no tokens is allowed by grammar but refused by all
		// properties and expanders; a custom property may be empty (CSS
		// Variables 1 §2.1: the "space toggle" --x: ;).
		if len(tokens) == 0 && !strings.HasPrefix(name, "--") {
			validationError("no value")
			continue
		}

		var (
			result expandedProperties
			err    error
		)
		if name == "all" {
			all, err := expandAll(tokens)
			if err != nil {
				validationError(err.Error())
				continue
			}
			for _, np := range all {
				ownDecls = append(ownDecls, Declaration{Name: np.name, Value: np.property, Important: declaration.Important})
			}
			continue
		}

		physicals := resolveLogical(name, tokens)
		if physicals == nil {
			validationError("invalid value count for logical shorthand")
			continue
		}
		for _, ld := range physicals {
			if sh := pr.NewShorthand(ld.name); sh != 0 {
				var expanded expandedProperties
				expanded, err = expanders[sh](baseURL, sh, ld.tokens)
				result = append(result, expanded...)
			} else {
				// validate without any expansion
				var r namedProperty
				r, err = validateNonShorthand(baseURL, ld.name, ld.tokens, false)
				result = append(result, r)
			}
			if err != nil {
				break
			}
		}

		if err != nil {
			validationError(err.Error())
			continue
		}

		important := declaration.Important

		for _, np := range result {
			ownDecls = append(ownDecls, Declaration{
				Name:      np.name,
				Value:     np.property,
				Important: important,
				Shorthand: np.shorthand,
			})
		}
	}

	out = append(out, KeyedDeclarations{selectors, ownDecls})

	return out, nil
}

// If `token` is [Ident], return its lower name.
// Otherwise return empty string.
func getKeyword(token Token) string {
	if ident, ok := token.(pa.Ident); ok {
		return utils.AsciiLower(ident.Value)
	}
	return ""
}

func getKeywordT(token Token) pr.Keyword {
	if ident, ok := token.(pa.Ident); ok {
		return pr.NewKeyword(utils.AsciiLower(ident.Value))
	}
	return 0
}

// If `tokens` is a 1-element list of [Ident], return its name.
// Otherwise return empty string.
func getSingleKeyword(tokens []Token) string {
	if len(tokens) == 1 {
		return getKeyword(tokens[0])
	}
	return ""
}

// absoluteURLs returns tokens with their relative url()s, at any depth,
// joined to baseURL (unchanged when it is empty, or on a join error).
func absoluteURLs(tokens []Token, baseURL string) []Token {
	if baseURL == "" {
		return tokens
	}
	join := func(u string) (string, bool) {
		if strings.HasPrefix(u, "#") {
			return u, false
		}
		joined, err := utils.SafeUrljoin(baseURL, u, false)
		return joined, err == nil && joined != u
	}
	var out []Token
	for i, token := range tokens {
		t, changed := pa.TransformTokens(token, func(t Token) (Token, bool, bool) {
			switch t := t.(type) {
			case pa.URL:
				if joined, ok := join(t.Value); ok {
					t.Value = joined
					return t, true, true
				}
				return t, false, true
			case pa.FunctionBlock:
				if utils.AsciiLower(t.Name) != "url" {
					return t, false, false
				}
				if len(t.Arguments) > 0 {
					if s, ok := t.Arguments[0].(pa.String); ok {
						if joined, ok := join(s.Value); ok {
							s.Value = joined
							args := slices.Clone(t.Arguments)
							args[0] = s
							t.Arguments = args
							return t, true, true
						}
					}
				}
				return t, false, true
			}
			return t, false, false
		})
		if changed {
			if out == nil {
				out = slices.Clone(tokens)
			}
			out[i] = t
		}
	}
	if out == nil {
		return tokens
	}
	return out
}

// expandAll expands the all shorthand (CSS Cascade 5 §3.2): a CSS-wide
// keyword for every property but direction, unicode-bidi and the custom
// properties.
func expandAll(tokens []Token) (expandedProperties, error) {
	keyword := getSingleKeyword(tokens)
	if _, ok := cssWideKeyword(keyword, 0); !ok {
		if len(tokens) == 1 && HasVar(tokens[0]) {
			return nil, errors.New("var() in the all shorthand is not supported")
		}
		return nil, errors.New("invalid value: all only takes a CSS-wide keyword")
	}
	props := make([]pr.KnownProp, 0, len(allValidators))
	for p := range allValidators {
		if p != pr.PDirection && p != pr.PUnicodeBidi {
			props = append(props, p)
		}
	}
	slices.Sort(props)
	out := make(expandedProperties, len(props))
	for i, p := range props {
		v, _ := cssWideKeyword(keyword, p)
		out[i] = namedProperty{name: pr.PropKey{KnownProp: p}, property: v}
	}
	return out, nil
}

// cssWideKeyword returns the value of the CSS-wide keyword (CSS Cascade 5
// §7.3) for prop, or false if keyword is not one: "initial", "inherit",
// "unset" (inherit for inherited properties, initial for the others),
// "revert" and "revert-layer" (resolved by the cascade).
func cssWideKeyword(keyword string, prop pr.KnownProp) (pr.DefaultValue, bool) {
	switch keyword {
	case "initial":
		return pr.Initial, true
	case "inherit":
		return pr.Inherit, true
	case "revert":
		return pr.Revert, true
	case "revert-layer":
		return pr.RevertLayer, true
	case "unset":
		if pr.IsInherited(prop) {
			return pr.Inherit, true
		}
		return pr.Initial, true
	}
	return 0, false
}

// If `tokens` is a 1-element list of [Ident], return its name.
// Otherwise return 0.
func getSingleKeywordT(tokens []Token) pr.Keyword {
	if len(tokens) == 1 {
		return getKeywordT(tokens[0])
	}
	return 0
}

// negative  = true, percentage = false
func getLength(token Token, negative, percentage bool) pr.Dimension {
	switch token := token.(type) {
	case pa.Percentage:
		if percentage && (negative || token.ValueF >= 0) {
			return pr.PercToD(token.ValueF)
		}
	case pa.Dimension:
		unit, isKnown := lengthUnits[strings.ToLower(string(token.Unit))]
		if isKnown && (negative || token.ValueF >= 0) {
			return pr.NewDim(pr.Float(token.ValueF), unit)
		}
	case pa.Number:
		if token.ValueF == 0 {
			return pr.NewDim(0, pr.Scalar)
		}
	}
	return pr.Dimension{}
}

// Return the value in radians of an <angle> token, or None.
func getAngle(token Token) (float32, bool) {
	if v, ok := contextFreeMath(token, pr.MathAngle); ok {
		return v, true
	}
	if dim, ok := token.(pa.Dimension); ok {
		unit, in := angleUnits[utils.AsciiLower(dim.Unit)]
		if in {
			return dim.ValueF * angleToRadians[unit], true
		}
	}
	// Per WP commit 5d4e6cec: legacy syntax allows a bare 0 (no unit)
	// to mean 0 angle. Used by Tailwind, see CSS Values 4 §6.4.
	if num, ok := token.(pa.Number); ok && num.ValueF == 0 {
		return 0, true
	}
	return 0, false
}

// Return the value in dppx of a <resolution> token, or false.
func getResolution(token Token) (float32, bool) {
	if v, ok := contextFreeMath(token, pr.MathResolution); ok {
		return v, true
	}
	if dim, ok := token.(pa.Dimension); ok {
		factor, in := resolutionToDppx[utils.AsciiLower(dim.Unit)]
		if in {
			return dim.ValueF * factor, true
		}
	}
	return 0, false
}

// @validator()
// @commaSeparatedList
// @singleKeyword
// “background-attachment“ property validation.
func _backgroundAttachment(tokens []Token) string {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "scroll", "fixed", "local":
		return keyword
	default:
		return ""
	}
}

func backgroundAttachment(tokens []Token, _ string) pr.CssProperty {
	var out pr.Strings
	for _, part := range pa.SplitOnComma(tokens) {
		part = pa.RemoveWhitespace(part)
		result := _backgroundAttachment(part)
		if result == "" {
			return nil
		}
		out = append(out, result)
	}
	return out
}

// @validator("background-color")
// @validator("border-top-color")
// @validator("border-right-color")
// @validator("border-bottom-color")
// @validator("border-left-color")
// @validator("column-rule-color")
// @singleToken
func otherColors(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		c := parseColorResolved(tokens[0])
		if !c.IsNone() {
			return pr.Color(c)
		}
	}
	return nil
}

// @validator()
// @singleToken
func outlineColor(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		token := tokens[0]
		if getKeyword(token) == "invert" {
			return pr.Color{Type: pa.ColorCurrentColor}
		} else {
			// Per WP commit 0af0b250: reject invalid colors instead
			// of silently passing through the zero color.
			c := parseColorResolved(token)
			if c.IsNone() {
				return nil
			}
			return pr.Color(c)
		}
	}
	return nil
}

// @validator()
// @singleKeyword
func borderCollapse(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "separate", "collapse":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “empty-cells“ property validation.
func emptyCells(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "show", "hide":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator("color")
// @singleToken
// “*-color“ && “color“ properties validation.
func color(tokens []Token, _ string) pr.DeclaredValue {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	switch result := parseColorResolved(token); result.Type {
	case pa.ColorCurrentColor:
		return pr.Inherit
	case pa.ColorRGBA, pa.ColorDependsOnCurrent: // see computedColor in webrender
		return pr.Color(result)
	default: // not a color: the declaration is invalid
		return nil
	}
}

// @validator("background-image", wantsBaseUrl=true)
// @commaSeparatedList
// @singleToken
func _backgroundImage(tokens []Token, baseURL string) (pr.Image, error) {
	if len(tokens) != 1 {
		return nil, nil
	}
	token := tokens[0]

	if getKeyword(token) == "none" {
		return pr.NoneImage{}, nil
	}
	return getImage(token, baseURL)
}

func backgroundImage(tokens []Token, baseURL string) (pr.CssProperty, error) {
	var out pr.Images
	for _, part := range pa.SplitOnComma(tokens) {
		part = pa.RemoveWhitespace(part)
		result, err := _backgroundImage(part, baseURL)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, nil
		}
		out = append(out, result)

	}
	return out, nil
}

var directionKeywords = map[[3]string]pr.DirectionType{
	// ("angle", radians)  0 upwards, then clockwise
	{"to", "top", ""}:    {Angle: 0},
	{"to", "right", ""}:  {Angle: pr.Fl(math.Pi) / 2},
	{"to", "bottom", ""}: {Angle: math.Pi},
	{"to", "left", ""}:   {Angle: math.Pi * 3 / 2},
	// ("corner", keyword)
	{"to", "top", "left"}:     {Corner: "top_left"},
	{"to", "left", "top"}:     {Corner: "top_left"},
	{"to", "top", "right"}:    {Corner: "top_right"},
	{"to", "right", "top"}:    {Corner: "top_right"},
	{"to", "bottom", "left"}:  {Corner: "bottom_left"},
	{"to", "left", "bottom"}:  {Corner: "bottom_left"},
	{"to", "bottom", "right"}: {Corner: "bottom_right"},
	{"to", "right", "bottom"}: {Corner: "bottom_right"},
}

// @validator("list-style-image", wantsBaseUrl=true)
// @singleToken
// “list-style-image“ property validation.
func listStyleImage(tokens []Token, baseURL string) (pr.CssProperty, error) {
	if len(tokens) != 1 {
		return nil, nil
	}
	token := tokens[0]

	if token.Kind() != pa.KFunctionBlock {
		if getKeyword(token) == "none" {
			return pr.NoneImage{}, nil
		}
		parsedUrl, _, err := getUrl(token, baseURL)
		if err != nil {
			return nil, err
		}
		if parsedUrl.Tag == pr.External {
			return pr.UrlImage(parsedUrl.S), nil
		}
	}
	return nil, nil
}

var centerKeywordFakeToken = pa.NewIdent("center", pa.Pos{})

// @validator(unstable=true)
func transformOrigin(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 3 {
		// Ignore third parameter as 3D transforms are ignored.
		tokens = tokens[:2]
	}
	return parse2dPosition(tokens)
}

// @validator()
// @commaSeparatedList
// “background-position“ and “object-position“ property validation.
// See http://drafts.csswg.org/csswg/css-backgrounds-3/#the-background-position
func backgroundPosition(tokens []Token, _ string) pr.CssProperty {
	out := centers(tokens)

	if len(out) == 0 {
		return nil
	}

	return out
}

func centers(tokens []Token) pr.Centers {
	var out pr.Centers
	for _, part := range pa.SplitOnComma(tokens) {
		result := parsePosition(pa.RemoveWhitespace(part))
		if result.IsNone() {
			return nil
		}
		out = append(out, result)
	}
	return out
}

// object-position property validation
func objectPosition(tokens []Token, _ string) pr.CssProperty {
	out := centers(tokens)

	if len(out) == 0 {
		return nil
	}
	return out[0]
}

// Common syntax of background-position and transform-origin.
func parse2dPosition(tokens []Token) pr.Point {
	if len(tokens) == 1 {
		tokens = []Token{tokens[0], centerKeywordFakeToken}
	} else if len(tokens) != 2 {
		return pr.Point{}
	}

	token1, token2 := tokens[0], tokens[1]
	length1 := getLengthDim(token1, true, true)
	length2 := getLengthDim(token2, true, true)
	if !length1.IsNone() && !length2.IsNone() {
		return pr.Point{length1, length2}
	}
	keyword1, keyword2 := getKeyword(token1), getKeyword(token2)
	if !length1.IsNone() && (keyword2 == "top" || keyword2 == "center" || keyword2 == "bottom") {
		return pr.Point{length1, backgroundPositionsPercentages[keyword2]}
	} else if !length2.IsNone() && (keyword1 == "left" || keyword1 == "center" || keyword1 == "right") {
		return pr.Point{backgroundPositionsPercentages[keyword1], length2}
	} else if (keyword1 == "left" || keyword1 == "center" || keyword1 == "right") &&
		(keyword2 == "top" || keyword2 == "center" || keyword2 == "bottom") {
		return pr.Point{backgroundPositionsPercentages[keyword1], backgroundPositionsPercentages[keyword2]}
	} else if (keyword1 == "top" || keyword1 == "center" || keyword1 == "bottom") &&
		(keyword2 == "left" || keyword2 == "center" || keyword2 == "right") {
		// Swap tokens. They need to be in (horizontal, vertical) order.
		return pr.Point{backgroundPositionsPercentages[keyword2], backgroundPositionsPercentages[keyword1]}
	}
	return pr.Point{}
}

// @validator()
// @commaSeparatedList
// “background-repeat“ property validation.
func _backgroundRepeat(tokens []Token) [2]string {
	keywords := make([]string, len(tokens))
	for index, token := range tokens {
		keywords[index] = getKeyword(token)
	}

	switch len(keywords) {
	case 1:
		switch keywords[0] {
		case "repeat-x":
			return [2]string{"repeat", "no-repeat"}
		case "repeat-y":
			return [2]string{"no-repeat", "repeat"}
		case "no-repeat", "repeat", "space", "round":
			return [2]string{keywords[0], keywords[0]}
		}
	case 2:
		for _, k := range keywords {
			if !(k == "no-repeat" || k == "repeat" || k == "space" || k == "round") {
				return [2]string{}
			}
		}
		// OK
		return [2]string{keywords[0], keywords[1]}
	}
	return [2]string{}
}

func backgroundRepeat(tokens []Token, _ string) pr.CssProperty {
	var out pr.Repeats
	for _, part := range pa.SplitOnComma(tokens) {
		result := _backgroundRepeat(pa.RemoveWhitespace(part))
		if result == [2]string{} {
			return nil
		}
		out = append(out, result)
	}
	return out
}

// @validator()
// @commaSeparatedList
// Validation for “background-size“.
func _backgroundSize(tokens []Token) pr.Size {
	switch len(tokens) {
	case 1:
		token := tokens[0]
		keyword := getKeyword(token)
		switch keyword {
		case "contain":
			return pr.Size{Tag: pr.Contain}
		case "cover":
			return pr.Size{Tag: pr.Cover}
		case "auto":
			return pr.Size{Width: pr.TagToV(pr.Auto), Height: pr.TagToV(pr.Auto)}
		}
		length := getLengthDim(token, false, true)
		if !length.IsNone() {
			return pr.Size{Width: length.Tagged(), Height: pr.TagToV(pr.Auto)}
		}
	case 2:
		var out pr.Size
		lengthW := getLengthDim(tokens[0], false, true)
		lengthH := getLengthDim(tokens[1], false, true)
		if !lengthW.IsNone() {
			out.Width = lengthW.Tagged()
		} else if getKeyword(tokens[0]) == "auto" {
			out.Width = pr.TagToV(pr.Auto)
		} else {
			return pr.Size{}
		}
		if !lengthH.IsNone() {
			out.Height = lengthH.Tagged()
		} else if getKeyword(tokens[1]) == "auto" {
			out.Height = pr.TagToV(pr.Auto)
		} else {
			return pr.Size{}
		}
		return out
	}
	return pr.Size{}
}

func backgroundSize(tokens []Token, _ string) pr.CssProperty {
	var out pr.Sizes
	for _, part := range pa.SplitOnComma(tokens) {
		result := _backgroundSize(pa.RemoveWhitespace(part))
		if (result == pr.Size{}) {
			return nil
		}
		out = append(out, result)
	}
	return out
}

// @validator("background-clip")
// @validator("background-origin")
// @commaSeparatedList
// @singleKeyword
// Validation for the “<box>“ type used in “background-clip“
//
//	and ``background-origin``.
func _box(tokens []Token) string {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "border-box", "padding-box", "content-box":
		return keyword
	default:
		return ""
	}
}

func box(tokens []Token, _ string) pr.CssProperty {
	var out pr.Strings
	for _, part := range pa.SplitOnComma(tokens) {
		result := _box(pa.RemoveWhitespace(part))
		if result == "" {
			return nil
		}
		out = append(out, result)
	}
	return out
}

func borderDims(tokens []Token, negative, percentage bool) pr.CssProperty {
	if len(tokens) != 1 && len(tokens) != 2 {
		return nil
	}
	axes := make([]pr.Dimension, len(tokens))
	for i, token := range tokens {
		dim := getLength(token, negative, percentage)
		if !dim.IsNone() {
			axes[i] = dim
			continue
		}
		if isMathFunction(token) {
			shape := pr.MathLength
			if percentage {
				shape = pr.MathLengthPercentage
			}
			// without percentages, the value resolves at computed-value
			// time (border-spacing), with them at layout time (border-radius)
			referTo := ""
			if percentage {
				referTo = "layout"
			}
			pm, err := parseMath(token, shape, referTo)
			if err != nil {
				return nil
			}
			pm.NonNegative = !negative
			axes[i] = pr.Dimension{Math: &pm}
			continue
		}
		return nil
	}
	if len(axes) == 1 {
		return pr.Point{axes[0], axes[0]}
	}
	return pr.Point{axes[0], axes[1]}
}

// @validator()
// Validator for the `border-spacing` property.
func borderSpacing(tokens []Token, _ string) pr.CssProperty {
	return borderDims(tokens, false, false) // no negative lengths (CSS 2 §17.6.1)
}

// @validator("border-top-right-radius")
// @validator("border-bottom-right-radius")
// @validator("border-bottom-left-radius")
// @validator("border-top-left-radius")
// Validator for the `border-*-radius` pr.
func borderCornerRadius(tokens []Token, _ string) pr.CssProperty {
	return borderDims(tokens, false, true)
}

// @validator("border-top-style")
// @validator("border-right-style")
// @validator("border-left-style")
// @validator("border-bottom-style")
// @validator("column-rule-style")
// @singleKeyword
// “border-*-style“ properties validation.
func borderStyle(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "none", "hidden", "dotted", "dashed", "double",
		"inset", "outset", "groove", "ridge", "solid":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator("break-before")
// @validator("break-after")
// @singleKeyword
// “break-before“ && “break-after“ properties validation.
func breakBeforeAfter(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "avoid", "avoid-page", "page", "left", "right",
		"recto", "verso", "avoid-column", "column", "always":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “break-inside“ property validation.
func breakInside(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "avoid", "avoid-page", "avoid-column":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “box-decoration-break“ property validation.
func boxDecorationBreak(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "slice", "clone":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “margin-break“ property validation.
func marginBreak(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "keep", "discard":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator(unstable=true)
// @singleToken
// “page“ property validation.
func page(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if ident, ok := token.(pa.Ident); ok {
		if utils.AsciiLower(ident.Value) == "auto" {
			return pr.Page("auto")
		}
		return pr.Page(ident.Value)
	}
	return nil
}

// @validator("bleed-left")
// @validator("bleed-right")
// @validator("bleed-top")
// @validator("bleed-bottom")
// @singleToken
// “bleed“ property validation.
func bleed(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if getKeyword(token) == "auto" {
		return pr.TaggedDim{Tag: pr.Auto}
	}
	// a <length>, or a math function giving one; anything else is invalid
	return getLengthOrCalc(token, true, false, "")
}

// @validator()
// “marks“ property validation.
func marks(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 2 {
		keywords := [2]string{getKeyword(tokens[0]), getKeyword(tokens[1])}
		if keywords == [2]string{"crop", "cross"} || keywords == [2]string{"cross", "crop"} {
			return pr.Marks{Crop: true, Cross: true}
		}
	} else if len(tokens) == 1 {
		keyword := getKeyword(tokens[0])
		switch keyword {
		case "crop":
			return pr.Marks{Crop: true}
		case "cross":
			return pr.Marks{Cross: true}
		case "none":
			return pr.Marks{}
		}
	}
	return nil
}

// @validator("outline-style")
// @singleKeyword
// “outline-style“ properties validation.
func outlineStyle(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "none", "dotted", "dashed", "double", "inset",
		"outset", "groove", "ridge", "solid":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator("border-top-width")
// @validator("border-right-width")
// @validator("border-left-width")
// @validator("border-bottom-width")
// @validator("column-rule-width")
// @validator("outline-width")
// @singleToken
// Border, column rule && outline widths properties validation.
func borderWidth(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if v := getLengthOrCalc(token, false, false, ""); v != nil {
		return v
	}
	tag := getKeywordT(token)
	if tag == pr.Thin || tag == pr.Medium || tag == pr.Thick {
		return pr.TaggedDim{Tag: tag}
	}
	return nil
}

// @single_token
func borderImageSource(tokens []Token, baseURL string) (pr.CssProperty, error) {
	if len(tokens) != 1 {
		return nil, nil
	}
	token := tokens[0]
	if getKeyword(token) == "none" {
		return pr.NoneImage{}, nil
	}
	return getImage(token, baseURL)
}

func borderImageSlice(tokens []Token, _ string) pr.CssProperty {
	var (
		values pr.Values
		fill   bool
	)
	for i, token := range tokens {
		// Don't use get_length() because a dimension with a unit is disallowed.
		if v, ok := token.(pa.Percentage); ok && v.ValueF >= 0 {
			values = append(values, pr.PercToV(v.ValueF))
		} else if v, ok := token.(pa.Number); ok && v.ValueF >= 0 {
			values = append(values, pr.FToV(v.ValueF))
		} else if getKeyword(token) == "fill" && !fill && (i == 0 || i == len(tokens)-1) {
			fill = true
			values = append(values, pr.TagToV(pr.Fill))
		} else {
			return nil
		}
	}
	if L := len(values); (fill && 2 <= L && L <= 5) || (1 <= L && L <= 4) {
		return values
	}
	return nil
}

func borderImageWidth(tokens []Token, _ string) pr.CssProperty {
	var values pr.Values
	for _, token := range tokens {
		if getKeyword(token) == "auto" {
			values = append(values, pr.TagToV(pr.Auto))
		} else if v, ok := token.(pa.Number); ok && v.ValueF >= 0 {
			values = append(values, pr.FToV(v.ValueF))
		} else {
			if length := getLength(token, false, true); !length.IsNone() {
				values = append(values, length.Tagged())
			} else {
				return nil
			}
		}
	}

	if L := len(values); 1 <= L && L <= 4 {
		return values
	}
	return nil
}

func borderImageOutset(tokens []Token, _ string) pr.CssProperty {
	var values pr.Values
	for _, token := range tokens {
		if v, ok := token.(pa.Number); ok && v.ValueF >= 0 {
			values = append(values, pr.FToV(v.ValueF))
		} else {
			if length := getLength(token, false, false); !length.IsNone() {
				values = append(values, length.Tagged())
			} else {
				return nil
			}
		}
	}
	if L := len(values); 1 <= L && L <= 4 {
		return values
	}
	return nil
}

func borderImageRepeat(tokens []Token, _ string) pr.CssProperty {
	if L := len(tokens); 1 <= L && L <= 2 {
		var keywords pr.Strings
		for _, token := range tokens {
			switch k := getKeyword(token); k {
			case "stretch", "repeat", "round", "space":
				keywords = append(keywords, k)
			default:
				return nil
			}
		}
		return keywords
	}
	return nil
}

// @property()
// @single_keyword
func maskBorderMode(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "luminance", "alpha":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// “column-width“ property validation.
func columnWidth(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if v := getLengthOrCalc(token, false, false, ""); v != nil {
		return v
	}
	keyword := getKeyword(token)
	if keyword == "auto" {
		return pr.TagToV(pr.Auto)
	}
	return nil
}

// @validator()
// @singleKeyword
// “column-span“ property validation.
func columnSpan(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "all", "none":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// Validation for the “box-sizing“ property from css3-ui
func boxSizing(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "padding-box", "border-box", "content-box":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “caption-side“ properties validation.
func captionSide(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "top", "bottom":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “clear“ property validation.
func clear(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "left", "right", "both", "none":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// Validation for the “clip“ property.
func clip(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	name, args := pa.ParseFunction(token)
	if name != "" {
		if name == "rect" && len(args) == 4 {
			var values pr.Values
			for _, arg := range args {
				if getKeyword(arg) == "auto" {
					values = append(values, pr.TagToV(pr.Auto))
				} else {
					length := getLength(arg, true, false)
					if !length.IsNone() {
						values = append(values, length.Tagged())
					}
				}
			}
			if len(values) == 4 {
				return values
			}
		}
	}
	if getKeyword(token) == "auto" {
		return pr.Values{}
	}
	return nil
}

// @validator(wantsBaseUrl=true)
// “content“ property validation.
func content(tokens []Token, baseURL string) (pr.CssProperty, error) {
	var token Token
	for len(tokens) > 0 {
		if len(tokens) >= 2 {
			if lit, ok := tokens[1].(pa.Literal); ok && lit.Value == "," {
				token, tokens = tokens[0], tokens[2:]
				im, err := getImage(token, baseURL)
				if err != nil {
					return nil, err
				}
				if im == nil {
					ur, atr, err := getUrl(token, baseURL)
					if err != nil || (ur.IsNone() && atr.IsNone()) {
						return nil, err
					}
				}
			} else {
				break
			}
		} else {
			break
		}
	}

	if len(tokens) == 0 {
		return nil, nil
	}
	if len(tokens) >= 3 {
		lit, ok := tokens[len(tokens)-2].(pa.Literal)
		if tokens[len(tokens)-1].Kind() == pa.KString && ok && lit.Value == "/" {
			// Ignore text for speech
			tokens = tokens[:len(tokens)-2]
		}
	}

	keyword := getSingleKeyword(tokens)
	if keyword == "normal" || keyword == "none" {
		return pr.SContent{String: keyword}, nil
	}
	l, err := getContentList(tokens, baseURL)
	if l == nil || err != nil {
		return nil, err
	}
	return pr.SContent{Contents: l}, nil
}

// @validator()
// “counter-increment“ property validation.
func counterIncrement(tokens []Token, _ string) (pr.CssProperty, error) {
	ci, err := counter(tokens, 1)
	if err != nil || ci == nil {
		return nil, err
	}
	return pr.SIntStrings{Values: ci}, nil
}

// “counter-reset“ property validation.
// “counter-set“ property validation.
func counterReset(tokens []Token, _ string) (pr.CssProperty, error) {
	ci, err := counter(tokens, 0)
	if err != nil || ci == nil {
		return nil, err
	}
	return pr.SIntStrings{Values: ci}, err
}

// “counter-increment“ && “counter-reset“ properties validation.
func counter(tokens []Token, defaultInteger int) ([]pr.IntString, error) {
	if getSingleKeyword(tokens) == "none" {
		return []pr.IntString{}, nil
	}
	if len(tokens) == 0 {
		return nil, errors.New("got an empty token list")
	}
	var (
		results []pr.IntString
		integer int
	)
	iter := pa.NewIter(tokens)
	token := iter.Next()
	for token != nil {
		ident, ok := token.(pa.Ident)
		if !ok {
			return nil, nil // expected a keyword here
		}
		counterName := ident.Value
		if counterName == "none" || counterName == "initial" || counterName == "inherit" {
			return nil, fmt.Errorf("invalid counter name: %s", counterName)
		}
		token = iter.Next()
		if number, ok := token.(pa.Number); ok && number.IsInt() { // implies token != nil
			// Found an integer. Use it and get the next token
			integer = number.Int()
			token = iter.Next()
		} else {
			// Not an integer. Might be the next counter name.
			// Keep `token` for the next loop iteration.
			integer = defaultInteger
		}
		results = append(results, pr.IntString{String: string(counterName), Int: integer})
	}
	return results, nil
}

// @validator("top")
// @validator("right")
// @validator("left")
// @validator("bottom")
// @validator("margin-top")
// @validator("margin-right")
// @validator("margin-bottom")
// @validator("margin-left")
// @singleToken
// “margin-*“ properties validation.
func lengthOrAuto(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	length := getLength(token, true, true)
	if !length.IsNone() {
		return pr.TaggedDim{Dimension: length}
	}
	if getKeyword(token) == "auto" {
		return pr.TaggedDim{Tag: pr.Auto}
	}
	return getLengthPercentageOrCalc(token, true, "layout")
}

// @validator("height")
// @validator("width")
// @singleToken
// Validation for the “width“ && “height“ pr.
func widthHeight(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	length := getLength(token, false, true)
	if !length.IsNone() {
		return length.Tagged()
	}
	if getKeyword(token) == "auto" {
		return pr.TagToV(pr.Auto)
	}
	return getLengthPercentageOrCalc(token, false, "layout")
}

// Validation for the “column-gap“ and "row-gap" property.
func gap(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	length := getLength(token, false, false)
	if !length.IsNone() {
		return length.Tagged()
	}
	if getKeyword(token) == "normal" {
		return pr.TagToV(pr.Normal)
	}
	// Per CSS Box Alignment 3: percentages are valid for gaps (resolved
	// against the container's content-box width).
	return getLengthPercentageOrCalc(token, false, "layout")
}

// @validator()
// @singleKeyword
// “column-fill“ property validation.
func columnFill(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "balance":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “direction“ property validation.
func direction(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	keyword := getKeywordT(tokens[0])
	switch keyword {
	case pr.Ltr, pr.Rtl:
		return pr.Keyword(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “display“ property validation.
func display(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeywordT(tokens)
	switch keyword {
	case pr.None, pr.TableCaption, pr.TableRowGroup, pr.TableCell,
		pr.TableHeaderGroup, pr.TableFooterGroup, pr.TableRow,
		pr.TableColumnGroup, pr.TableColumn:
		return pr.Display{Outside: keyword}
	case pr.InlineTable:
		return pr.Display{Outside: pr.Inline, Inside: pr.Table}
	case pr.InlineFlex:
		return pr.Display{Outside: pr.Inline, Inside: pr.Flex}
	case pr.InlineGrid:
		return pr.Display{Outside: pr.Inline, Inside: pr.Grid}
	case pr.InlineBlock:
		return pr.Display{Outside: pr.Inline, Inside: pr.FlowRoot}
	}

	var outside, inside, listItem pr.Keyword
	for _, token := range tokens {
		value := getKeywordT(token)
		switch value {
		case pr.Block, pr.Inline:
			if outside != 0 {
				return nil
			}
			outside = value
		case pr.Flow, pr.FlowRoot, pr.Table, pr.Flex, pr.Grid:
			if inside != 0 {
				return nil
			}
			inside = value
		case pr.ListItem:
			if listItem != 0 {
				return nil
			}
			listItem = value
		default:
			return nil
		}
	}

	if outside == 0 {
		outside = pr.Block
	}
	if inside == 0 {
		inside = pr.Flow
	}

	if listItem == 0 || (inside == pr.Flow || inside == pr.FlowRoot) {
		return pr.Display{Outside: outside, Inside: inside, ListItem: listItem}
	}

	return nil
}

// @validator("float")
// @singleKeyword
// “float“ property validation.
func float(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "left", "right", "footnote", "none":
		return pr.String(keyword)
	default:
		return nil
	}
}

func _fontFamily(tokens []Token) string {
	if len(tokens) == 0 {
		return ""
	}
	if tt, ok := tokens[0].(pa.String); len(tokens) == 1 && ok {
		return tt.Value
	} else if len(tokens) > 0 {
		var values []string
		for _, token := range tokens {
			if tt, ok := token.(pa.Ident); ok {
				values = append(values, string(tt.Value))
			} else {
				return ""
			}
		}
		return strings.Join(values, " ")
	}
	return ""
}

// @validator()
// @commaSeparatedList
// “font-family“ property validation.
func fontFamily(tokens []Token, _ string) pr.CssProperty {
	var out pr.Strings
	for _, part := range pa.SplitOnComma(tokens) {
		result := _fontFamily(pa.RemoveWhitespace(part))
		if result == "" {
			return nil
		}
		out = append(out, result)
	}
	return out
}

// @validator()
// @singleKeyword
func fontKerning(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "normal", "none":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
func fontLanguageOverride(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	keyword := getKeyword(token)
	if keyword == "normal" {
		return pr.String(keyword)
	}
	if tt, ok := token.(pa.String); ok {
		return pr.String(tt.Value)
	}
	return nil
}

func parseFontVariant(tokens []Token, all utils.Set, couples [][]string) pr.SStrings {
	var values []string
	isInValues := func(s string, vs []string) bool {
		for _, v := range vs {
			if s == v {
				return true
			}
		}
		return false
	}
	for _, token := range tokens {
		ident, isIdent := token.(pa.Ident)
		if !isIdent {
			return pr.SStrings{}
		}
		identValue := string(ident.Value)
		if all.Has(identValue) {
			var concurrentValues []string
			for _, couple := range couples {
				if isInValues(identValue, couple) {
					concurrentValues = couple
					break
				}
			}
			for _, value := range concurrentValues {
				if isInValues(value, values) {
					return pr.SStrings{}
				}
			}
			values = append(values, identValue)
		} else {
			return pr.SStrings{}
		}
	}
	if len(values) > 0 {
		return pr.SStrings{Strings: values}
	}
	return pr.SStrings{}
}

// @validator()
func fontVariantLigatures(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		keyword := getKeyword(tokens[0])
		if keyword == "normal" || keyword == "none" {
			return pr.SStrings{String: keyword}
		}
	}
	ss := parseFontVariant(tokens, allLigaturesValues, couplesLigatures)
	if ss.IsNone() {
		return nil
	}
	return ss
}

// @validator()
// @singleKeyword
func fontVariantPosition(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "normal", "sub", "super":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
func fontVariantCaps(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "normal", "small-caps", "all-small-caps", "petite-caps",
		"all-petite-caps", "unicase", "titling-caps":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
func fontVariantNumeric(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		keyword := getKeyword(tokens[0])
		if keyword == "normal" {
			return pr.SStrings{String: keyword}
		}
	}
	ss := parseFontVariant(tokens, allNumericValues, couplesNumeric)
	if ss.IsNone() {
		return nil
	}
	return ss
}

// @validator()
// “font-feature-settings“ property validation.
func fontFeatureSettings(tokens []Token, _ string) pr.CssProperty {
	s, ok := _fontFeatureSettings(tokens)
	if !ok {
		return nil
	}
	return s
}

func _fontFeatureSettings(tokens []Token) (pr.FontFeatures, bool) {
	if len(tokens) == 1 && getKeyword(tokens[0]) == "normal" {
		return nil, true
	}

	fontFeatureSettingsList := func(tokens []Token) pr.FontFeature {
		var token Token
		feature, value := [4]byte{}, uint32(0)

		if len(tokens) == 2 {
			tokens, token = tokens[0:1], tokens[1]
			switch tt := token.(type) {
			case pa.Ident:
				if tt.Value == "on" {
					value = 1
				} else {
					value = 0
				}
			case pa.Number:
				if tt.IsInt() && tt.Int() >= 0 {
					value = uint32(tt.Int())
				}
			}
		} else if len(tokens) == 1 {
			value = 1
		}

		if len(tokens) == 1 {
			token = tokens[0]
			if tt, ok := token.(pa.String); ok && len(tt.Value) == 4 {
				for _, letter := range tt.Value {
					if !(0x20 <= letter && letter <= 0x7f) {
						return pr.FontFeature{}
					}
				}
				copy(feature[:], tt.Value)
			}
		}

		return pr.FontFeature{Tag: feature, Value: value}
	}

	var out pr.FontFeatures
	for _, part := range pa.SplitOnComma(tokens) {
		result := fontFeatureSettingsList(pa.RemoveWhitespace(part))
		if result.Tag == ([4]byte{}) { // invalid tag
			return nil, false
		}
		out = append(out, result)
	}
	return out, len(out) != 0
}

// @validator()
// @singleKeyword
func fontVariantAlternates(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	// TODO: support other values
	// See https://www.w3.org/TR/css-fonts-3/#font-variant-caps-prop
	switch keyword {
	case "normal", "historical-forms":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
func fontVariantEastAsian(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		keyword := getKeyword(tokens[0])
		if keyword == "normal" {
			return pr.SStrings{String: keyword}
		}
	}
	ss := parseFontVariant(tokens, allEastAsianValues, couplesEastAsian)
	if ss.IsNone() {
		return nil
	}
	return ss
}

// @property()
// “font-variation-settings“ property validation.
func fontVariationSettings(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 && getKeyword(tokens[0]) == "normal" {
		return pr.SFloatStrings{String: "normal"}
	}
	// @comma_separated_list
	fontVariationSettingsList := func(tokens []Token) (pr.FloatString, bool) {
		if len(tokens) == 2 {
			keyV, ok1 := tokens[0].(pa.String)
			valueV, ok2 := tokens[1].(pa.Number)
			if ok1 && ok2 {
				return pr.FloatString{String: keyV.Value, Float: valueV.ValueF}, true
			}
		}
		return pr.FloatString{}, false
	}

	var out pr.SFloatStrings
	for _, part := range pa.SplitOnComma(tokens) {
		result, ok := fontVariationSettingsList(pa.RemoveWhitespace(part))
		if !ok {
			return nil
		}
		out.Values = append(out.Values, result)
	}
	return out
}

// @validator()
// @singleToken
// “font-size“ property validation.
func fontSize(tokens []Token, _ string) (pr.CssProperty, error) {
	if len(tokens) != 1 {
		return nil, nil
	}
	token := tokens[0]
	length := getLength(token, false, true)
	if !length.IsNone() {
		return length.Tagged(), nil
	}
	tag := getKeywordT(token)
	if isIn := pr.XxSmall <= tag && tag <= pr.XxLarge; isIn || tag == pr.Smaller || tag == pr.Larger {
		return pr.TaggedDim{Tag: tag}, nil
	}
	// Percentages on font-size resolve against the parent's font-size.
	// We defer that to the fontSize computer function which has access
	// to the parent style.
	return getLengthPercentageOrCalc(token, false, "parent-font-size"), nil
}

// @validator()
// @singleKeyword
// “font-style“ property validation.
func fontStyle(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "normal", "italic", "oblique":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// Validation for the “font-stretch“ property.
func fontStretch(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "ultra-condensed", "extra-condensed", "condensed", "semi-condensed",
		"normal", "semi-expanded", "expanded", "extra-expanded", "ultra-expanded":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// “font-weight“ property validation.
func fontWeight(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	keyword := getKeyword(token)
	if keyword == "normal" || keyword == "bold" || keyword == "bolder" || keyword == "lighter" {
		return pr.IntString{String: keyword}
	}
	if number, ok := token.(pa.Number); ok {
		intValue := number.Int()
		if number.IsInt() && (intValue == 100 || intValue == 200 || intValue == 300 || intValue == 400 || intValue == 500 || intValue == 600 || intValue == 700 || intValue == 800 || intValue == 900) {
			return pr.IntString{Int: intValue}
		}
	}
	return nil
}

// @validator()
// @single_keyword
func objectFit(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	// TODO: Figure out what the spec means by "'scale-down' flag".
	//  As of this writing, neither Firefox nor chrome support
	//  anything other than a single keyword as is done here.
	switch keyword {
	case "fill", "contain", "cover", "none", "scale-down":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator(unstable=true)
// @singleToken
func imageResolution(tokens []Token, _ string) pr.CssProperty {
	// TODO: support "snap" && "from-image"
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	value, ok := getResolution(token)
	if !ok {
		return nil
	}
	return pr.FToV(pr.Fl(value))
}

// @validator("letter-spacing")
// @validator("word-spacing")
// @singleToken
// Validation for “letter-spacing“ && “word-spacing“.
func spacing(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if getKeyword(token) == "normal" {
		return pr.TaggedDim{Tag: pr.Normal}
	}
	return getLengthOrCalc(token, true, false, "")
}

// @validator()
// @singleToken
// “line-height“ property validation.
func lineHeight(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if getKeyword(token) == "normal" {
		return pr.TagToV(pr.Normal)
	}

	switch tt := token.(type) {
	case pa.Number:
		if tt.ValueF >= 0 {
			return pr.NewDim(pr.Float(tt.ValueF), pr.Scalar).Tagged()
		}
	case pa.Percentage:
		if tt.ValueF >= 0 {
			return pr.PercToV(tt.ValueF)
		}
	case pa.Dimension:
		if tt.ValueF >= 0 {
			l := getLength(token, true, false)
			if l.IsNone() {
				return nil
			}
			return l.Tagged()
		}
	case pa.FunctionBlock:
		if isMathFunction(tt) {
			// a <length-percentage>, or a <number> (the multiplier)
			pm, err := parseMath(tt, pr.MathLengthPercentage, "font-size")
			if err != nil {
				pm, err = parseMath(tt, pr.MathNumber, "")
			}
			if err == nil {
				pm.NonNegative = true
				return pm
			}
		}
	}
	return nil
}

// @validator()
// @singleKeyword
// “list-style-position“ property validation.
func listStylePosition(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "inside", "outside":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// “list-style-type“ property validation.
func listStyleType(tokens []Token, _ string) pr.CssProperty {
	out, ok := listStyleType_(tokens)
	if ok {
		return out
	}
	return nil
}

func listStyleType_(tokens []Token) (out pr.CounterStyleID, ok bool) {
	if len(tokens) != 1 {
		return out, false
	}
	token := tokens[0]
	switch token := token.(type) {
	case pa.Ident:
		return pr.CounterStyleID{Name: string(token.Value)}, true
	case pa.String:
		return pr.CounterStyleID{Type: "string", Name: token.Value}, true
	case pa.FunctionBlock:
		if token.Name != "symbols" {
			return out, false
		}
		functionArguments := pa.RemoveWhitespace(token.Arguments)
		if len(functionArguments) == 0 {
			return out, false
		}
		arguments := []string{"symbolic"}
		if arg0, ok := functionArguments[0].(pa.Ident); ok {
			if arg0.Value == "cyclic" || arg0.Value == "numeric" || arg0.Value == "alphabetic" || arg0.Value == "symbolic" || arg0.Value == "fixed" {
				arguments = []string{string(arg0.Value)}
				functionArguments = functionArguments[1:]
			} else {
				return out, false
			}
		}

		if len(functionArguments) == 0 {
			return out, false
		}

		for _, arg := range functionArguments {
			if str, ok := arg.(pa.String); ok {
				arguments = append(arguments, str.Value)
			} else {
				return out, false
			}
		}

		if arguments[0] == "alphabetic" || arguments[0] == "numeric" {
			if len(arguments) < 3 {
				return out, false
			}
		}
		return pr.CounterStyleID{Type: "symbols()", Name: arguments[0], Symbols: arguments[1:]}, true
	default:
		return out, false
	}
}

// @validator("min-width")
// @validator("min-height")
// @singleToken
// “min-width“ && “min-height“ properties validation.
func minWidthHeight(tokens []Token, _ string) pr.CssProperty {
	// See https://www.w3.org/TR/css-flexbox-1/#min-size-auto
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	keyword := getKeyword(token)
	if keyword == "auto" {
		return pr.TaggedDim{Tag: pr.Auto}
	}

	dim, ok := dimension(tokens)
	if ok {
		return pr.TaggedDim{Dimension: dim}
	}

	return getLengthPercentageOrCalc(token, false, "layout")
}

// @validator("padding-top")
// @validator("padding-right")
// @validator("padding-bottom")
// @validator("padding-left")
// @singleToken
// “padding-*“ properties validation.
func lengthOrPercentage(tokens []Token, _ string) pr.CssProperty {
	dim, ok := dimension(tokens)
	if ok {
		return dim.Tagged()
	}
	if len(tokens) != 1 {
		return nil
	}
	return getLengthPercentageOrCalc(tokens[0], false, "layout")
}

func dimension(tokens []Token) (pr.Dimension, bool) {
	if len(tokens) != 1 {
		return pr.Dimension{}, false
	}
	token := tokens[0]
	l := getLength(token, false, true)
	if l.IsNone() {
		return pr.Dimension{}, false
	}
	return l, true
}

// @validator("max-width")
// @validator("max-height")
// @singleToken
// Validation for max-width && max-height
func maxWidthHeight(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	length := getLength(token, false, true)
	if !length.IsNone() {
		return length.Tagged()
	}
	if getKeyword(token) == "none" {
		return pr.NewDim(pr.Inf, pr.Px).Tagged()
	}
	return getLengthPercentageOrCalc(token, false, "layout")
}

// @validator()
// @singleToken
// Validation for the “opacity“ property.
func opacity(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if number, ok := token.(pa.Number); ok {
		return pr.Float(min(1, max(0, number.ValueF)))
	} else if perc, ok := token.(pa.Percentage); ok {
		return pr.Float(min(1, max(0, perc.ValueF/100)))
	} else if isMathFunction(token) {
		pm, err := parseMath(token, pr.MathNumber, "")
		if err == nil {
			return pm
		}
	}

	return nil
}

// @validator()
// @singleToken
// Validation for the “z-index“ property.
func zIndex(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if getKeyword(token) == "auto" {
		return pr.IntString{String: "auto"}
	}
	if number, ok := token.(pa.Number); ok {
		if number.IsInt() {
			return pr.IntString{Int: number.Int()}
		}
		return nil
	}
	// math: an <integer>, rounded at computed-value time
	if isMathFunction(token) {
		if pm, err := parseMath(token, pr.MathNumber, ""); err == nil {
			return pm
		}
	}
	return nil
}

// @validator("orphans")
// @validator("widows")
// @singleToken
// Validation for the “orphans“ && “widows“ pr.
func orphansWidows(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if number, ok := token.(pa.Number); ok {
		value := number.Int()
		if number.IsInt() && value >= 1 {
			return pr.Int(value)
		}
	}
	if v, ok := contextFreeInteger(token); ok {
		return pr.Int(max(1, v)) // clamped to the range [1, ∞)
	}
	return nil
}

// @validator()
// @singleToken
// Validation for the “column-count“ property.
func columnCount(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if number, ok := token.(pa.Number); ok {
		value := number.Int()
		if number.IsInt() && value >= 1 {
			return pr.IntString{Int: value}
		}
	}
	if getKeyword(token) == "auto" {
		return pr.IntString{String: "auto"}
	}
	if isMathFunction(token) {
		// an <integer> expression only holds numbers: it does not depend
		// on the element, and resolves now, rounded to the nearest integer
		// and clamped to the range [1, ∞) (CSS Values 4 §10.9, §10.12)
		if pm, err := parseMath(token, pr.MathInteger, ""); err == nil {
			if v, err := EvalMath(pm, probeContext); err == nil && !math.IsNaN(float64(v)) {
				return pr.IntString{Int: max(1, int(math.Round(float64(min(v, math.MaxInt32)))))}
			}
		}
	}
	return nil
}

// @validator()
// @singleKeyword
// Validation for the “overflow“ property.
func overflow(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "visible", "hidden", "scroll":
		return pr.String(keyword)
	default:
		return nil
	}
}

// Validation for the “word-break“ property.
func wordBreak(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "normal", "break-all":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @single_keyword
// Validation for the “text-overflow“ property.
func textOverflow(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "clip", "ellipsis":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// “position“ property validation.
func position(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if fn, ok := token.(pa.FunctionBlock); ok && fn.Name == "running" && len(fn.Arguments) == 1 {
		if ident, ok := (fn.Arguments)[0].(pa.Ident); ok {
			return pr.BoolString{Bool: true, String: string(ident.Value)}
		}
	}
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "static", "relative", "absolute", "fixed":
		return pr.BoolString{String: keyword}
	default:
		return nil
	}
}

// @validator()
// “quotes“ property validation.
func quotes(tokens []Token, _ string) pr.CssProperty {
	var opens, closes []string
	if len(tokens) == 1 {
		keyword := getKeyword(tokens[0])
		switch keyword {
		case "auto":
			return pr.Quotes{Tag: pr.Auto}
		case "none":
			return pr.Quotes{Tag: pr.None}
		}
	}
	if len(tokens) > 0 && len(tokens)%2 == 0 {
		// Separate open && close quotes.
		// eg.  ("«", "»", "“", "”")  -> (("«", "“"), ("»", "”"))
		for i := 0; i < len(tokens); i += 2 {
			open, ok1 := tokens[i].(pa.String)
			close_, ok2 := tokens[i+1].(pa.String)
			if ok1 && ok2 {
				opens = append(opens, open.Value)
				closes = append(closes, close_.Value)
			} else {
				return nil
			}
		}
		return pr.Quotes{Open: opens, Close: closes}
	}
	return nil
}

// @validator()
// @singleKeyword
// Validation for the “table-layout“ property
func tableLayout(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "fixed", "auto":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “text-align-all“ property validation.
func textAlignAll(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "left", "right", "center", "justify", "start", "end":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “text-align-last“ property validation.
func textAlignLast(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "left", "right", "center", "justify", "start", "end":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// “text-decoration-line“ property validation.
func textDecorationLine(tokens []Token, _ string) pr.CssProperty {
	uniqKeywords := utils.Set{}
	var out pr.Decorations
	valid := true
	for _, token := range tokens {
		keyword := getKeyword(token)
		switch keyword {
		case "underline":
			out |= pr.Underline
		case "overline":
			out |= pr.Overline
		case "line-through":
			out |= pr.LineThrough
		case "blink":
			out |= pr.Blink
		default:
			valid = false
		}
		uniqKeywords.Add(keyword)
	}
	if _, in := uniqKeywords["none"]; len(uniqKeywords) == 1 && in { // then uniqKeywords == {"none"}
		return out
	}
	if valid && len(uniqKeywords) == len(tokens) {
		return out
	}
	return nil
}

// @validator()
// @singleKeyword
// “text-decoration-style“ property validation.
func textDecorationStyle(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "solid", "double", "dotted", "dashed", "wavy":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// “text-decoration-thickness“ property validation (CSS Text Decoration L4).
// Accepts: auto | from-font | <length> | <percentage> (percentage of 1em).
func textDecorationThickness(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	length := getLength(token, true, true)
	if !length.IsNone() {
		return length.Tagged()
	}
	switch keyword := getKeyword(token); keyword {
	case "auto":
		return pr.TaggedDim{Tag: pr.Auto}
	case "from-font":
		return pr.TaggedDim{Tag: pr.FromFont}
	}
	return getLengthPercentageOrCalc(token, true, "font-size")
}

// @validator()
// @singleToken
// “text-underline-offset“ property validation (CSS Text Decoration L4).
// Accepts: auto | <length> | <percentage> (percentage of 1em).
func textUnderlineOffset(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	length := getLength(token, true, true)
	if !length.IsNone() {
		return length.Tagged()
	}
	if getKeyword(token) == "auto" {
		return pr.TaggedDim{Tag: pr.Auto}
	}
	return getLengthPercentageOrCalc(token, true, "font-size")
}

// @validator()
// @singleToken
// “text-indent“ property validation.
func textIndent(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	return getLengthPercentageOrCalc(tokens[0], true, "layout")
}

// @validator()
// @singleKeyword
// “text-align“ property validation.
func textTransform(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "none", "uppercase", "lowercase", "capitalize", "full-width":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// Validation for the “vertical-align“ property
func verticalAlign(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	length := getLength(token, true, true)
	if !length.IsNone() {
		return length.Tagged()
	}
	// math: a percentage refers to the line height of the element, known
	// at computed-value time
	if v := getLengthOrCalc(token, true, true, "line-height"); v != nil {
		return v
	}
	switch tag := getKeywordT(token); tag {
	case pr.Baseline, pr.Middle, pr.Sub, pr.Super, pr.TextTop, pr.TextBottom, pr.Top, pr.Bottom:
		return pr.TaggedDim{Tag: tag}
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “visibility“ property validation.
func visibility(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "visible", "hidden", "collapse":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “white-space“ property validation.
func whiteSpace(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeywordT(tokens)
	switch keyword {
	case pr.Normal, pr.Pre, pr.Nowrap, pr.PreWrap, pr.PreLine:
		return keyword
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “overflow-wrap“ property validation.
func overflowWrap(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "anywhere", "normal", "break-word":
		return pr.String(keyword)
	default:
		return nil
	}
}

// Validation for “footnote-display“.
func footnoteDisplay(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "block", "inline", "compact":
		return pr.String(keyword)
	default:
		return nil
	}
}

// Validation for “footnote-policy“.
func footnotePolicy(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "line", "block":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleToken
// “flex-basis“ property validation.
func flexBasis(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	basis := widthHeight(tokens, "")
	if basis != nil {
		return basis
	}
	if getKeyword(token) == "content" {
		return pr.TagToV(pr.Content)
	}
	return nil
}

// @validator()
// @singleKeyword
// “flex-direction“ property validation.
func flexDirection(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "row", "row-reverse", "column", "column-reverse":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator("flex-grow")
// @validator("flex-shrink")
// @singleToken
func flexGrowShrink(tokens []Token, _ string) pr.CssProperty {
	if f, ok := _flexGrowShrink(tokens); ok {
		return pr.Float(f)
	}
	if len(tokens) == 1 && isMathFunction(tokens[0]) {
		return getNumberOrCalc(tokens[0])
	}
	return nil
}

func _flexGrowShrink(tokens []Token) (pr.Fl, bool) {
	if len(tokens) != 1 {
		return 0, false
	}
	token := tokens[0]
	if number, ok := token.(pa.Number); ok && number.ValueF >= 0 {
		return number.ValueF, true
	}
	if isMathFunction(token) {
		// a <number> expression does not depend on the element: it
		// resolves now, clamped to [0, ∞)
		if pm, err := parseMath(token, pr.MathNumber, ""); err == nil {
			if v, err := EvalMath(pm, probeContext); err == nil {
				return max(0, v), true
			}
		}
	}
	return 0, false
}

// Parse “inflexible-breadth“.
func parseInflexibleBreadth(token Token) pr.TaggedDim {
	keyword := getKeyword(token)
	switch keyword {
	case "auto":
		return pr.TagToV(pr.Auto)
	case "min-content":
		return pr.TagToV(pr.MinContent)
	case "max-content":
		return pr.TagToV(pr.MaxContent)
	case "":
		length := getLength(token, false, true)
		if !length.IsNone() {
			return length.Tagged()
		}
	}

	return pr.TaggedDim{}
}

// Parse “track-breadth“.
func parseTrackBreadth(token Token) pr.TaggedDim {
	if dim, ok := token.(pa.Dimension); ok && dim.ValueF >= 0 && dim.Unit == "fr" {
		return pr.NewDim(pr.Float(dim.ValueF), pr.Fr).Tagged()
	}
	return parseInflexibleBreadth(token)
}

// Parse “track-size“.
func parseTrackSize(token Token) pr.GridDims {
	trackBreadth := parseTrackBreadth(token)
	if !trackBreadth.IsNone() {
		return pr.NewGridDimsValue(trackBreadth)
	}
	name, args := pa.ParseFunction(token)
	if name == "minmax" {
		if len(args) == 2 {
			inflexibleBreadth := parseInflexibleBreadth(args[0])
			trackBreadth := parseTrackBreadth(args[1])
			if !inflexibleBreadth.IsNone() && !trackBreadth.IsNone() {
				return pr.NewGridDimsMinmax(inflexibleBreadth, trackBreadth)
			}
		}
	} else if name == "fit-content" {
		if len(args) == 1 {
			length := getLength(args[0], false, true)
			if !length.IsNone() {
				return pr.NewGridDimsFitcontent(length)
			}
		}
	}

	return pr.GridDims{}
}

// Parse “fixed-size“.
func parseFixedSize(token Token) pr.GridDims {
	length := getLength(token, false, true)
	if !length.IsNone() {
		return pr.NewGridDimsValue(length.Tagged())
	}
	name, args := pa.ParseFunction(token)
	if name == "minmax" && len(args) == 2 {
		length := getLength(args[0], false, true)
		if !length.IsNone() {
			trackBreadth := parseTrackBreadth(args[1])
			if !trackBreadth.IsNone() {
				return pr.NewGridDimsMinmax(length.Tagged(), trackBreadth)
			}
		}
		keyword := getKeyword(args[0])
		if keyword == "min-content" || keyword == "max-content" || keyword == "auto" || !length.IsNone() {
			fixedBreadth := getLength(args[1], false, true)
			if !fixedBreadth.IsNone() {
				v1 := length.Tagged()
				if v1.IsNone() {
					v1 = pr.TagToV(pr.NewKeyword(keyword))
				}
				return pr.NewGridDimsMinmax(v1, fixedBreadth.Tagged())
			}
		}
	}

	return pr.GridDims{}
}

// parse “line-names“, returning nil if invalid,
// but an empty list for '[]'
func parseLineNames(arg Token) []string {
	if arg, ok := arg.(pa.SquareBracketsBlock); ok {
		names := []string{}
		for _, token := range arg.Arguments {
			if ident, ok := token.(pa.Ident); ok {
				names = append(names, ident.Value)
			} else if _, ok := token.(pa.Whitespace); ok {
				continue
			} else {
				return nil
			}
		}
		return names
	}

	return nil
}

// @property("grid-auto-columns")
// @property("grid-auto-rows")
// “grid-auto-columns“ and “grid-auto-rows“ properties validation.
func gridAuto(tokens []Token, _ string) pr.CssProperty {
	var returnTokens pr.GridAuto
	for _, token := range tokens {
		trackSize := parseTrackSize(token)
		if trackSize.IsNone() {
			return nil
		}
		returnTokens = append(returnTokens, trackSize)
	}
	return returnTokens
}

// @property()
// “grid-auto-flow“ property validation.
func gridAutoFlow(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		keyword := getKeyword(tokens[0])
		switch keyword {
		case "row", "column":
			return pr.Strings{keyword}
		case "dense":
			return pr.Strings{keyword, "row"}
		}
	} else if len(tokens) == 2 {
		keywords := [2]string{getKeyword(tokens[0]), getKeyword(tokens[1])}
		switch keywords {
		case [2]string{"dense", "row"}, [2]string{"dense", "column"}, [2]string{"row", "dense"}, [2]string{"column", "dense"}:
			return pr.Strings(keywords[:])
		}
	}
	return nil
}

func parseRepeat(token Token, acceptAutoFit bool) (int, bool) {
	if nb, ok := token.(pa.Number); ok && nb.IsInt() && nb.ValueF >= 1 {
		return nb.Int(), true
	}
	switch getKeyword(token) {
	case "auto-fill":
		return pr.RepeatAutoFill, true
	case "auto-fit":
		if acceptAutoFit {
			return pr.RepeatAutoFit, true
		}
	}
	return 0, false
}

// [tokens] start right after 'subgrid'
func parseSubgrid(tokens []Token) ([]pr.GridSpec, bool) {
	var subgrid []pr.GridSpec
	for _, token := range tokens {
		lineNames := parseLineNames(token)
		if lineNames != nil {
			subgrid = append(subgrid, pr.GridNames(lineNames))
			continue
		}

		name, args := pa.ParseFunction(token)
		if !(name == "repeat" && len(args) >= 2) {
			return nil, false
		}

		reapeat, ok := parseRepeat(args[0], false)
		if !ok { // invalid
			return nil, false
		}
		var names [][]string
		for _, arg := range args[1:] {
			lineNames := parseLineNames(arg)
			if lineNames != nil {
				names = append(names, lineNames)
			}
		}
		subgrid = append(subgrid, pr.GridNameRepeat{Repeat: reapeat, Names: names})
	}
	return subgrid, true
}

// @property("grid-template-columns")
// @property("grid-template-rows")
// “grid-template-columns“ and “grid-template-rows“ validation.
func gridTemplate(tokens []Token, _ string) pr.CssProperty {
	if v, ok := gridTemplateImpl(tokens); ok {
		return v
	}
	return nil
}

func gridTemplateImpl(tokens []Token) (out pr.GridTemplate, _ bool) {
	if len(tokens) == 0 {
		return out, false
	}
	if len(tokens) == 1 && getKeyword(tokens[0]) == "none" {
		return pr.GridTemplate{Tag: pr.None}, true
	}
	if getKeyword(tokens[0]) == "subgrid" {
		if subgrid, ok := parseSubgrid(tokens[1:]); ok {
			return pr.GridTemplate{Tag: pr.Subgrid, Names: subgrid}, true
		}
		return out, false
	}

	var (
		returnTokens       []pr.GridSpec
		includesAutoRepeat = false
		includesTrack      = false
		lastIsLineName     = false
	)
	for _, token := range tokens {
		lineNames := parseLineNames(token)
		if lineNames != nil {
			if lastIsLineName {
				return out, false
			}
			lastIsLineName = true
			returnTokens = append(returnTokens, pr.GridNames(lineNames))
			continue
		}
		fixedSize := parseFixedSize(token)
		if !fixedSize.IsNone() {
			if !lastIsLineName {
				returnTokens = append(returnTokens, pr.GridNames{})
			}
			lastIsLineName = false
			returnTokens = append(returnTokens, fixedSize)
			continue
		}
		trackSize := parseTrackSize(token)
		if !trackSize.IsNone() {
			if !lastIsLineName {
				returnTokens = append(returnTokens, pr.GridNames{})
			}
			lastIsLineName = false
			returnTokens = append(returnTokens, trackSize)
			includesTrack = true
			continue
		}
		name, args := pa.ParseFunction(token)
		if name == "repeat" && len(args) >= 2 {
			number, ok := parseRepeat(args[0], true)
			if !ok {
				return out, false
			}
			if number <= -1 { // auto-repeat
				if includesAutoRepeat {
					return out, false
				}
				includesAutoRepeat = true
			}

			var (
				namesAndSizes        []pr.GridSpec
				repeatLastIsLineName = false
			)
			for _, arg := range args[1:] {
				lineNames = parseLineNames(arg)
				if lineNames != nil {
					if repeatLastIsLineName {
						return out, false
					}
					namesAndSizes = append(namesAndSizes, pr.GridNames(lineNames))
					repeatLastIsLineName = true
					continue
				}
				// fixed-repead
				fixedSize = parseFixedSize(arg)
				if !fixedSize.IsNone() {
					if !repeatLastIsLineName {
						namesAndSizes = append(namesAndSizes, pr.GridNames{})
					}
					repeatLastIsLineName = false
					namesAndSizes = append(namesAndSizes, fixedSize)
					continue
				}
				// track-repeat
				trackSize = parseTrackSize(arg)
				if !trackSize.IsNone() {
					includesTrack = true
					if !repeatLastIsLineName {
						namesAndSizes = append(namesAndSizes, pr.GridNames{})
					}
					repeatLastIsLineName = false
					namesAndSizes = append(namesAndSizes, trackSize)
					continue
				}
				return out, false
			}
			if !lastIsLineName {
				returnTokens = append(returnTokens, pr.GridNames{})
			}
			lastIsLineName = false
			if !repeatLastIsLineName {
				namesAndSizes = append(namesAndSizes, pr.GridNames{})
			}
			returnTokens = append(returnTokens, pr.GridRepeat{Names: namesAndSizes, Repeat: number})
			continue
		}
		return out, false
	}
	if includesAutoRepeat && includesTrack {
		return out, false
	}
	if !lastIsLineName {
		returnTokens = append(returnTokens, pr.GridNames{})
	}
	return pr.GridTemplate{Names: returnTokens}, true
}

// @property()
// “grid-template-areas“ property validation.
func gridTemplateAreas(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 && getKeyword(tokens[0]) == "none" {
		return pr.GridTemplateAreas{}
	}
	var gridAreas pr.GridTemplateAreas
	for _, token := range tokens {
		s, ok := token.(pa.String)
		if !ok {
			return nil
		}
		componentValues := pa.Tokenize([]byte(s.Value), true)
		var (
			row       []string
			lastIsDot = false
		)
		for _, value := range componentValues {
			switch value := value.(type) {
			case pa.Ident:
				row = append(row, value.Value)
				lastIsDot = false
			case pa.Literal:
				if value.Value != "." {
					return nil
				}
				if lastIsDot {
					continue
				}
				row = append(row, "")
				lastIsDot = true
			case pa.Whitespace:
				lastIsDot = false
			default:
				return nil
			}
		}
		if len(row) == 0 {
			return nil
		}
		gridAreas = append(gridAreas, row)
	}

	// check row / column have the same sizes
	L := len(gridAreas[0])
	for _, other := range gridAreas {
		if len(other) != L {
			return nil
		}
	}
	// check areas are continuous rectangles
	coordinates := make(map[[2]int]bool)
	areas := make(map[string]bool)
	for y, row := range gridAreas {
		for x, area := range row {
			if in := coordinates[[2]int{x, y}]; in || area == "" {
				continue
			}
			if in := areas[area]; in {
				return nil
			}
			areas[area] = true
			coordinates[[2]int{x, y}] = true
			nx := x
			hasBroken := false
			for index, narea := range row[x+1:] {
				nx = index + x + 1
				if narea != area {
					hasBroken = true
					break
				}
				coordinates[[2]int{nx, y}] = true
			}
			if !hasBroken {
				nx += 1
			}
			for ny := y + 1; ny < len(gridAreas); ny++ {
				nrow := gridAreas[ny]
				if set := utils.NewSet(nrow[x:nx]...); len(set) == 1 && set.Has(area) {
					for nnx := x; nnx < nx; nnx++ {
						coordinates[[2]int{nnx, ny}] = true
					}
				} else {
					break
				}
			}
		}
	}
	return gridAreas
}

// @property("grid-row-start")
// @property("grid-row-end")
// @property("grid-column-start")
// @property("grid-column-end")
// “grid-[row|column]-[start—end]“ properties validation.
func gridLine(tokens []Token, _ string) pr.CssProperty {
	v, ok := gridLineImpl(tokens)
	if ok {
		return v
	}
	return nil
}

func gridLineImpl(tokens []Token) (pr.GridLine, bool) {
	if len(tokens) == 1 {
		token := tokens[0]
		if keyword := getKeyword(token); keyword != "" {
			if keyword == "auto" {
				return pr.GridLine{Tag: pr.Auto}, true
			} else if keyword != "span" {
				return pr.GridLine{Ident: token.(pa.Ident).Value}, true
			}
		} else if number, ok := token.(pa.Number); ok && number.IsInt() && number.ValueF != 0 {
			return pr.GridLine{Val: number.Int()}, true
		}
		return pr.GridLine{}, false
	}
	var (
		number int
		ident  string
		span   pr.Keyword
	)

	for _, token := range tokens {
		if keyword := getKeyword(token); keyword != "" {
			if keyword == "auto" {
				return pr.GridLine{}, false
			}
			if keyword == "span" {
				if span == 0 {
					span = pr.Span
					continue
				}
			} else if ident == "" {
				ident = token.(pa.Ident).Value
				continue
			}
		} else if nbT, ok := token.(pa.Number); ok && nbT.IsInt() && nbT.ValueF != 0 {
			if number == 0 {
				number = nbT.Int()
				continue
			}
		}
		return pr.GridLine{}, false
	}
	if span != 0 {
		if number < 0 {
			return pr.GridLine{}, false
		} else if ident != "" || number != 0 {
			return pr.GridLine{Tag: span, Val: number, Ident: ident}, true
		}
	} else if number != 0 {
		return pr.GridLine{Tag: span, Val: number, Ident: ident}, true
	}

	return pr.GridLine{}, false
}

// @validator()
// @singleToken
func order(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if number, ok := token.(pa.Number); ok && number.IsInt() {
		return pr.Int(number.Int())
	}
	if v, ok := contextFreeInteger(token); ok {
		return pr.Int(v)
	}
	return nil
}

// @validator()
// @singleKeyword
// “flex-wrap“ property validation.
func flexWrap(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "nowrap", "wrap", "wrap-reverse":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator()
// @singleKeyword
// “justify-content“ property validation.
func justifyContent(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		switch keyword := getKeywordT(tokens[0]); keyword {
		case pr.Center, pr.SpaceBetween, pr.SpaceAround, pr.SpaceEvenly,
			pr.Stretch, pr.Normal, pr.FlexStart, pr.FlexEnd,
			pr.Start, pr.End, pr.Left, pr.Right:
			return pr.JustifyOrAlign{keyword}
		}
	} else if len(tokens) == 2 {
		kw1, kw2 := getKeywordT(tokens[0]), getKeywordT(tokens[1])
		if kw1 == pr.Safe || kw1 == pr.Unsafe {
			switch kw2 {
			case pr.Center, pr.Start, pr.End, pr.FlexStart, pr.FlexEnd, pr.Left,
				pr.Right:
				return pr.JustifyOrAlign{kw1, kw2}
			}
		}
	}
	return nil
}

// @validator()
// “align-items“ property validation.
func justifyItems(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		switch keyword := getKeywordT(tokens[0]); keyword {
		case pr.Normal, pr.Stretch, pr.Center, pr.Start, pr.End, pr.SelfStart,
			pr.SelfEnd, pr.FlexStart, pr.FlexEnd, pr.Left, pr.Right,
			pr.Legacy:
			return pr.JustifyOrAlign{keyword}
		case pr.Baseline:
			return pr.JustifyOrAlign{pr.First, keyword}
		}
	} else if len(tokens) == 2 {
		kw1, kw2 := getKeywordT(tokens[0]), getKeywordT(tokens[1])
		if kw1 == pr.Safe || kw1 == pr.Unsafe {
			switch kw2 {
			case pr.Center, pr.Start, pr.End, pr.SelfStart, pr.SelfEnd,
				pr.FlexStart, pr.FlexEnd, pr.Left, pr.Right:
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw1 == pr.Baseline {
			if kw2 == pr.First || kw2 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw2 == pr.Baseline {
			if kw1 == pr.First || kw1 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw1 == pr.Legacy {
			if kw2 == pr.Left || kw2 == pr.Right || kw2 == pr.Center {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw2 == pr.Legacy {
			if kw1 == pr.Left || kw1 == pr.Right || kw1 == pr.Center {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		}
	}
	return nil
}

// @validator()
// align-items property validation.
func justifySelf(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		switch keyword := getKeywordT(tokens[0]); keyword {
		case pr.Auto, pr.Normal, pr.Stretch, pr.Center, pr.Start, pr.End,
			pr.SelfStart, pr.SelfEnd, pr.FlexStart, pr.FlexEnd, pr.Left,
			pr.Right:
			return pr.JustifyOrAlign{keyword}
		case pr.Baseline:
			return pr.JustifyOrAlign{pr.First, keyword}
		}
	} else if len(tokens) == 2 {
		kw1, kw2 := getKeywordT(tokens[0]), getKeywordT(tokens[1])
		if kw1 == pr.Safe || kw1 == pr.Unsafe {
			switch kw2 {
			case pr.Center, pr.Start, pr.End, pr.SelfStart, pr.SelfEnd,
				pr.FlexStart, pr.FlexEnd, pr.Left, pr.Right:
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw1 == pr.Baseline {
			if kw2 == pr.First || kw2 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw2 == pr.Baseline {
			if kw1 == pr.First || kw1 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		}
	}
	return nil
}

// @validator()
// align-items property validation.
func alignItems(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		switch keyword := getKeywordT(tokens[0]); keyword {
		case pr.Normal, pr.Stretch, pr.Center, pr.Start, pr.End, pr.SelfStart,
			pr.SelfEnd, pr.FlexStart, pr.FlexEnd:
			return pr.JustifyOrAlign{keyword}
		case pr.Baseline:
			return pr.JustifyOrAlign{pr.First, keyword}
		}
	} else if len(tokens) == 2 {
		kw1, kw2 := getKeywordT(tokens[0]), getKeywordT(tokens[1])
		if kw1 == pr.Safe || kw1 == pr.Unsafe {
			switch kw2 {
			case pr.Center, pr.Start, pr.End, pr.SelfStart, pr.SelfEnd,
				pr.FlexStart, pr.FlexEnd:
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw1 == pr.Baseline {
			if kw2 == pr.First || kw2 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw2 == pr.Baseline {
			if kw1 == pr.First || kw1 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		}
	}
	return nil
}

// @validator()
// @singleKeyword
// align-self property validation.
func alignSelf(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		switch keyword := getKeywordT(tokens[0]); keyword {
		case pr.Auto, pr.Normal, pr.Stretch, pr.Center, pr.Start, pr.End,
			pr.SelfStart, pr.SelfEnd, pr.FlexStart, pr.FlexEnd:
			return pr.JustifyOrAlign{keyword}
		case pr.Baseline:
			return pr.JustifyOrAlign{pr.First, keyword}
		}
	} else if len(tokens) == 2 {
		kw1, kw2 := getKeywordT(tokens[0]), getKeywordT(tokens[1])
		if kw1 == pr.Safe || kw1 == pr.Unsafe {
			switch kw2 {
			case pr.Center, pr.Start, pr.End, pr.SelfStart, pr.SelfEnd,
				pr.FlexStart, pr.FlexEnd:
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw1 == pr.Baseline {
			if kw2 == pr.First || kw2 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw2 == pr.Baseline {
			if kw1 == pr.First || kw1 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		}
	}
	return nil
}

// @validator()
// align-content property validation.
func alignContent(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) == 1 {
		switch keyword := getKeywordT(tokens[0]); keyword {
		case pr.Center, pr.SpaceBetween, pr.SpaceAround, pr.SpaceEvenly,
			pr.Stretch, pr.Normal, pr.FlexStart, pr.FlexEnd,
			pr.Start, pr.End:
			return pr.JustifyOrAlign{keyword}
		case pr.Baseline:
			return pr.JustifyOrAlign{pr.First, keyword}
		}
	} else if len(tokens) == 2 {
		kw1, kw2 := getKeywordT(tokens[0]), getKeywordT(tokens[1])
		if kw1 == pr.Safe || kw1 == pr.Unsafe {
			switch kw2 {
			case pr.Center, pr.Start, pr.End, pr.FlexStart, pr.FlexEnd:
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw1 == pr.Baseline {
			if kw2 == pr.First || kw2 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		} else if kw2 == pr.Baseline {
			if kw1 == pr.First || kw1 == pr.Last {
				return pr.JustifyOrAlign{kw1, kw2}
			}
		}
	}
	return nil
}

// @validator(unstable=true)
// @singleKeyword
// Validation for “image-rendering“.
func imageRendering(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "crisp-edges", "pixelated":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @property(unstable=True)
// Validation for “image-orientation“.
func imageOrientation(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	if keyword == "none" || keyword == "from-image" {
		return pr.SBoolFloat{String: keyword}
	}
	var (
		angle             pr.Fl
		flip              bool
		hasAngle, hasFlip bool
	)
	for _, token := range tokens {
		keyword = getKeyword(token)
		if keyword == "flip" {
			if hasFlip {
				return nil
			}
			flip, hasFlip = true, true
			continue
		}
		if !hasAngle {
			var ok bool
			angle, ok = getAngle(token)
			hasAngle = true
			if ok {
				continue
			}
		}
		return nil
	}

	return pr.SBoolFloat{Float: angle, Bool: flip}
}

// @validator(unstable=true)
// “size“ property validation.
// See http://www.w3.org/TR/css3-page/#page-size-prop
// usesViewportUnit reports whether a viewport unit appears in token, at any
// depth (in a math function).
func usesViewportUnit(token Token) bool {
	found := false
	pa.TransformTokens(token, func(t Token) (Token, bool, bool) {
		if d, ok := t.(pa.Dimension); ok {
			if u, ok := LengthUnit(utils.AsciiLower(d.Unit)); ok && u.IsViewport() {
				found = true
			}
		}
		return t, false, found
	})
	return found
}

func size(tokens []Token, _ string) pr.CssProperty {
	var (
		lengths        []pr.Dimension
		keywords       []string
		lengthsNotNone = true
	)
	for _, token := range tokens {
		length, keyword := getLength(token, false, false), getKeyword(token)
		if pm, ok := getLengthOrCalc(token, false, false, "").(pr.PendingMath); ok {
			length = pr.Dimension{Math: &pm} // resolved at computed-value time
		}
		if length.Unit.IsViewport() || usesViewportUnit(token) {
			// The page box is what viewport units resolve *against*;
			// using one to define it is circular. Reject the whole
			// declaration rather than pick an arbitrary base.
			return nil
		}
		lengthsNotNone = lengthsNotNone && !length.IsNone()
		lengths = append(lengths, length)
		keywords = append(keywords, keyword)
	}

	if lengthsNotNone {
		if len(lengths) == 1 {
			return pr.Point{lengths[0], lengths[0]}
		} else if len(lengths) == 2 {
			return pr.Point{lengths[0], lengths[1]}
		}
	}

	if len(keywords) == 1 {
		keyword := keywords[0]
		if psize, in := pr.PageSize(keyword); in {
			return psize
		} else if keyword == "auto" || keyword == "portrait" {
			return pr.A4()
		} else if keyword == "landscape" {
			return pr.Point{pr.A4()[1], pr.A4()[0]}
		}
	}

	if len(keywords) == 2 {
		var orientation, pageSize string
		if keywords[0] == "portrait" || keywords[0] == "landscape" {
			orientation, pageSize = keywords[0], keywords[1]
		} else if keywords[1] == "portrait" || keywords[1] == "landscape" {
			pageSize, orientation = keywords[0], keywords[1]
		}
		if widthHeight, in := pr.PageSize(pageSize); in {
			if orientation == "portrait" {
				return widthHeight
			} else {
				return pr.Point{widthHeight[1], widthHeight[0]}
			}
		}
	}
	return nil
}

// @validator(proprietary=true)
// @singleToken
// Validation for “anchor“.
func anchor(tokens []Token, _ string) (out pr.CssProperty) {
	if len(tokens) != 1 {
		return
	}
	token := tokens[0]
	if getKeyword(token) == "none" {
		return pr.String("none")
	}
	name, args := pa.ParseFunction(token)
	if name != "" {
		if len(args) == 1 {
			if ident, ok := args[0].(pa.Ident); ok && name == "attr" {
				return pr.AttrData{Name: string(ident.Value)}
			}
		}
	}
	return
}

// @validator(proprietary=true, wantsBaseUrl=true)
// @singleToken
// Validation for “link“.
func link(tokens []Token, baseURL string) (out pr.CssProperty, err error) {
	if len(tokens) != 1 {
		return
	}
	token := tokens[0]
	if getKeyword(token) == "none" {
		return pr.TaggedString{Tag: pr.None}, nil
	}

	parsedUrl, _, err := getUrl(token, baseURL)
	if err != nil {
		return
	}
	if !parsedUrl.IsNone() {
		return parsedUrl, nil
	}
	name, args := pa.ParseFunction(token)
	if name == "attr" && len(args) == 1 {
		if ident, ok := args[0].(pa.Ident); ok {
			return pr.TaggedString{Tag: pr.Attr, S: string(ident.Value)}, nil
		}
	}
	return
}

// @validator()
// @singleToken
// Validation for “tab-size“.
// See https://www.w3.org/TR/css-text-3/#tab-size
func tabSize(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if number, ok := token.(pa.Number); ok {
		// a <number> (CSS Text 3 §4.2), multiple of the space width
		if number.ValueF >= 0 {
			return pr.NewDim(pr.Float(number.ValueF), 0).Tagged()
		}
		return nil
	}
	if isMathFunction(token) {
		// a number (multiple of the space width) or a length
		if pm, err := parseMath(token, pr.MathNumber, ""); err == nil {
			return pm
		}
		return getLengthOrCalc(token, false, false, "")
	}
	if length := getLength(token, false, false); !length.IsNone() {
		return length.Tagged()
	}
	return nil
}

// @validator(unstable=true)
// @singleToken
// Validation for “hyphens“.
func hyphens(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	keyword := getKeyword(token)
	switch keyword {
	case "none", "manual", "auto":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator(unstable=true)
// @singleToken
// Validation for “hyphenate-character“.
func hyphenateCharacter(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	keyword := getKeyword(token)
	if keyword == "auto" {
		return pr.String("‐")
	} else if str, ok := token.(pa.String); ok {
		return pr.String(str.Value)
	}
	return nil
}

// @validator(unstable=true)
// @singleToken
// Validation for “hyphenate-limit-zone“.
func hyphenateLimitZone(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if d := getLength(token, false, true); !d.IsNone() {
		return d.Tagged()
	}
	// math, resolved at computed-value time: a length, or a percentage of
	// the line kept as a percentage for the text layout. An expression
	// mixing both is rejected: it could only resolve at layout time.
	if v := getLengthOrCalc(token, false, false, ""); v != nil {
		return v
	}
	if isMathFunction(token) {
		if pm, err := parseMath(token, pr.MathPercentage, ""); err == nil {
			pm.NonNegative = true
			return pm
		}
	}
	return nil
}

// @validator(unstable=true)
// Validation for “hyphenate-limit-chars“.
func hyphenateLimitChars(tokens []Token, _ string) pr.CssProperty {
	switch len(tokens) {
	case 1:
		token := tokens[0]
		keyword := getKeyword(token)
		if keyword == "auto" {
			return pr.Limits{Total: 5, Left: 2, Right: 2}
		} else if number, ok := token.(pa.Number); ok && number.IsInt() {
			return pr.Limits{Total: number.Int(), Left: 2, Right: 2}
		}
	case 2:
		total, left := tokens[0], tokens[1]
		totalKeyword := getKeyword(total)
		leftKeyword := getKeyword(left)
		if totalNumber, ok := total.(pa.Number); ok && totalNumber.IsInt() {
			if leftNumber, ok := left.(pa.Number); ok && leftNumber.IsInt() {
				return pr.Limits{Total: totalNumber.Int(), Left: leftNumber.Int(), Right: leftNumber.Int()}
			} else if leftKeyword == "auto" {
				return pr.Limits{Total: totalNumber.Int(), Left: 2, Right: 2}
			}
		} else if totalKeyword == "auto" {
			if leftNumber, ok := left.(pa.Number); ok && leftNumber.IsInt() {
				return pr.Limits{Total: 5, Left: leftNumber.Int(), Right: leftNumber.Int()}
			} else if leftKeyword == "auto" {
				return pr.Limits{Total: 5, Left: 2, Right: 2}
			}
		}
	case 3:
		total, left, right := tokens[0], tokens[1], tokens[2]
		totalNumber, okT := total.(pa.Number)
		leftNumber, okL := left.(pa.Number)
		rightNumber, okR := right.(pa.Number)
		if ((okT && totalNumber.IsInt()) || getKeyword(total) == "auto") &&
			((okL && leftNumber.IsInt()) || getKeyword(left) == "auto") &&
			((okR && rightNumber.IsInt()) || getKeyword(right) == "auto") {
			totalInt := 5
			if okT {
				totalInt = totalNumber.Int()
			}
			leftInt := 2
			if okL {
				leftInt = leftNumber.Int()
			}
			rightInt := 2
			if okR {
				rightInt = rightNumber.Int()
			}
			return pr.Limits{Total: totalInt, Left: leftInt, Right: rightInt}
		}
	}
	return nil
}

// @validator(proprietary=true)
// @singleToken
// Validation for “lang“.
func lang(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if getKeyword(token) == "none" {
		return pr.TaggedString{Tag: pr.None}
	}
	name, args := pa.ParseFunction(token)
	if name != "" {
		if len(args) == 1 {
			if ident, ok := args[0].(pa.Ident); ok && name == "attr" {
				return pr.TaggedString{Tag: pr.Attr, S: ident.Value}
			}
		}
	} else if str, ok := token.(pa.String); ok {
		return pr.TaggedString{S: str.Value}
	}
	return nil
}

// @validator(unstable=true)
// Validation for “bookmark-label“.
func bookmarkLabel(tokens []Token, baseURL string) (out pr.CssProperty, err error) {
	parsedTokens := make(pr.ContentProperties, len(tokens))
	for index, v := range tokens {
		parsedTokens[index], err = getContentListToken(v, baseURL)
		if err != nil {
			return nil, err
		}
		if parsedTokens[index].IsNone() {
			return nil, nil
		}
	}
	return parsedTokens, nil
}

// @validator(unstable=true)
// @singleToken
// Validation for “bookmark-level“.
func bookmarkLevel(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if number, ok := token.(pa.Number); ok && number.IsInt() && number.Int() >= 1 {
		return pr.TaggedInt{I: number.Int()}
	} else if getKeyword(token) == "none" {
		return pr.TaggedInt{Tag: pr.None}
	}
	return nil
}

// @validator(unstable=True)
// @single_keyword
// Validation for “bookmark-state“.
func bookmarkState(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "open", "closed":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @validator(unstable=true)
// @commaSeparatedList
// Validation for “string-set“.
func _stringSet(tokens []Token, baseURL string) (out pr.SContent, err error) {
	// Spec asks for strings after custom keywords, but we allow content-lists
	if len(tokens) >= 2 {
		varName := getCustomIdent(tokens[0])
		if varName == "" {
			return
		}
		parsedTokens := make([]pr.ContentProperty, len(tokens)-1)
		for i, token := range tokens[1:] {
			parsedTokens[i], err = getContentListToken(token, baseURL)
			if err != nil {
				return
			}
			if parsedTokens[i].IsNone() {
				return
			}
		}
		return pr.SContent{String: varName, Contents: parsedTokens}, nil
	} else if len(tokens) > 0 && getKeyword(tokens[0]) == "none" {
		return pr.SContent{String: "none"}, nil
	}
	return
}

func stringSet(tokens []Token, baseURL string) (pr.CssProperty, error) {
	var out pr.StringSet
	for _, part := range pa.SplitOnComma(tokens) {
		result, err := _stringSet(pa.RemoveWhitespace(part), baseURL)
		if err != nil {
			return nil, err
		}
		if result.IsNone() {
			return nil, nil
		}
		out.Contents = append(out.Contents, result)
	}
	return out, nil
}

// @validator()
func transform(tokens []Token, _ string) (pr.CssProperty, error) {
	if getSingleKeyword(tokens) == "none" {
		return pr.Transforms{}, nil
	}
	out := make(pr.Transforms, len(tokens))
	var err error
	for index, v := range tokens {
		out[index], err = transformFunction(v)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// @property()
// @single_token
// “box-ellipsis“ property validation.
func blockEllipsis(tokens []Token, _ string) pr.CssProperty {
	if v, ok := blockEllipsis_(tokens); ok {
		return v
	}
	return nil
}

func blockEllipsis_(tokens []Token) (out pr.TaggedString, ok bool) {
	if len(tokens) != 1 {
		return
	}
	token := tokens[0]
	if str, ok := token.(pa.String); ok {
		return pr.TaggedString{S: str.Value}, true
	}
	switch keyword := getKeyword(token); keyword {
	case "none":
		return pr.TaggedString{Tag: pr.None}, true
	case "auto":
		return pr.TaggedString{Tag: pr.Auto}, true
	}
	return
}

// transformLength reads a <length-percentage> argument of a transform
// function, math included (resolved at layout time, against the box).
func transformLength(token Token) pr.Dimension { return getLengthDim(token, true, true) }

// transformNumber reads a <number> argument of a transform function, math
// included.
func transformNumber(token Token) (float32, bool) {
	if number, ok := token.(pa.Number); ok {
		return number.ValueF, true
	}
	return contextFreeMath(token, pr.MathNumber)
}

func transformFunction(token Token) (pr.Transform, error) {
	name, args := pa.ParseFunction(token)
	if name == "" {
		return pr.Transform{}, errInvalidValue
	}

	lengths, values := make([]pr.Dimension, len(args)), make([]pr.Dimension, len(args))
	isAllNumber, isAllLengths := true, true
	for index, a := range args {
		lengths[index] = transformLength(a)
		isAllLengths = isAllLengths && !lengths[index].IsNone()
		if v, ok := transformNumber(a); ok {
			values[index] = pr.FToD(v)
		} else {
			isAllNumber = false
		}
	}
	switch len(args) {
	case 1:
		angle, notNone := getAngle(args[0])
		length := transformLength(args[0])
		switch name {
		case "rotate":
			// Per WP commit 5d4e6cec: 0 (with or without unit) is a
			// valid angle, so don't reject it here.
			if notNone {
				return pr.Transform{Kind: pr.Rotate, Dimensions: []pr.Dimension{pr.FToD(pr.Fl(angle))}}, nil
			}
		case "skewx", "skew":
			if notNone {
				return pr.Transform{Kind: pr.Skew, Dimensions: []pr.Dimension{pr.FToD(pr.Fl(angle)), pr.ZeroPixels}}, nil
			}
		case "skewy":
			if notNone {
				return pr.Transform{Kind: pr.Skew, Dimensions: []pr.Dimension{pr.ZeroPixels, pr.FToD(pr.Fl(angle))}}, nil
			}
		case "translatex", "translate":
			if !length.IsNone() {
				return pr.Transform{Kind: pr.Translate, Dimensions: []pr.Dimension{length, pr.ZeroPixels}}, nil
			}
		case "translatey":
			if !length.IsNone() {
				return pr.Transform{Kind: pr.Translate, Dimensions: []pr.Dimension{pr.ZeroPixels, length}}, nil
			}
		case "scalex":
			if number, ok := transformNumber(args[0]); ok {
				return pr.Transform{Kind: pr.Scale, Dimensions: []pr.Dimension{pr.FToD(number), pr.FToD(1.)}}, nil
			}
		case "scaley":
			if number, ok := transformNumber(args[0]); ok {
				return pr.Transform{Kind: pr.Scale, Dimensions: []pr.Dimension{pr.FToD(1.), pr.FToD(number)}}, nil
			}
		case "scale":
			if number, ok := transformNumber(args[0]); ok {
				return pr.Transform{Kind: pr.Scale, Dimensions: []pr.Dimension{pr.FToD(number), pr.FToD(number)}}, nil
			}
		}
	case 2:
		if name == "scale" && isAllNumber {
			return pr.Transform{Kind: pr.Scale, Dimensions: values}, nil
		}
		if name == "translate" && isAllLengths {
			return pr.Transform{Kind: pr.Translate, Dimensions: lengths}, nil
		}
	case 6:
		if name == "matrix" && isAllNumber {
			return pr.Transform{Kind: pr.Matrix, Dimensions: values}, nil
		}
	}
	return pr.Transform{}, errInvalidValue
}

func maxLines(tokens []Token, _ string) pr.CssProperty {
	if len(tokens) != 1 {
		return nil
	}
	token := tokens[0]
	if token, ok := token.(pa.Number); ok {
		if token.IsInt() {
			return pr.TaggedInt{I: token.Int()}
		}
	}
	if keyword := getKeyword(token); keyword == "none" {
		return pr.TaggedInt{Tag: pr.None}
	}
	return nil
}

func continue_(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "discard":
		return pr.String(keyword)
	default:
		return nil
	}
}

// @property()
// @single_token
// “appearance“ property validation.
func appearance(tokens []Token, _ string) pr.CssProperty {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "auto", "none":
		return pr.String(keyword)
	default:
		return nil
	}
}

// parseSingleShadow parses one shadow value (shared by box-shadow and text-shadow).
func parseSingleShadow(tokens []Token, allowSpread, allowInset bool) (pr.Shadow, bool) {
	var (
		lengths  []pr.Dimension
		color    pr.Color
		colorSet bool
		inset    bool
	)

	for _, token := range tokens {
		if allowInset && getKeyword(token) == "inset" {
			if inset {
				return pr.Shadow{}, false
			}
			inset = true
			continue
		}

		if !colorSet {
			c := parseColorResolved(token)
			if c.Type != 0 {
				color = pr.Color(c)
				colorSet = true
				continue
			}
		}

		l := getLength(token, true, false)
		if l.IsNone() {
			return pr.Shadow{}, false
		}
		lengths = append(lengths, l)
	}

	if len(lengths) < 2 {
		return pr.Shadow{}, false
	}

	maxLengths := 3
	if allowSpread {
		maxLengths = 4
	}
	if len(lengths) > maxLengths {
		return pr.Shadow{}, false
	}

	if !colorSet {
		color = pr.Color(pa.ParseColorString("currentcolor"))
	}

	var blur, spread pr.Dimension
	if len(lengths) >= 3 {
		blur = lengths[2]
		if blur.Value < 0 {
			return pr.Shadow{}, false
		}
	}
	if len(lengths) >= 4 {
		spread = lengths[3]
	}

	return pr.Shadow{
		OffsetX: lengths[0],
		OffsetY: lengths[1],
		Blur:    blur,
		Spread:  spread,
		Color:   color,
		Inset:   inset,
	}, true
}

func boxShadow(tokens []Token, _ string) pr.CssProperty {
	if getSingleKeyword(tokens) == "none" {
		return pr.Shadows{}
	}

	var shadows pr.Shadows
	for _, part := range pa.SplitOnComma(tokens) {
		part = pa.RemoveWhitespace(part)
		if len(part) == 0 {
			return nil
		}
		shadow, ok := parseSingleShadow(part, true, true)
		if !ok {
			return nil
		}
		shadows = append(shadows, shadow)
	}
	return shadows
}

func textShadow(tokens []Token, _ string) pr.CssProperty {
	if getSingleKeyword(tokens) == "none" {
		return pr.Shadows{}
	}

	var shadows pr.Shadows
	for _, part := range pa.SplitOnComma(tokens) {
		part = pa.RemoveWhitespace(part)
		if len(part) == 0 {
			return nil
		}
		shadow, ok := parseSingleShadow(part, false, false)
		if !ok {
			return nil
		}
		shadows = append(shadows, shadow)
	}
	return shadows
}
