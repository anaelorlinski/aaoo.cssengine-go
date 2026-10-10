package validation

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/counters"
	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/logger"

	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// Validate descriptors, currently used for @font-face rules.
// See https://www.w3.org/TR/css-fonts-3/#font-resources.

// :copyright: Copyright 2011-2016 Simon Sapin && contributors, see AUTHORS.
// :license: BSD, see LICENSE for details.
// (WeasyPrint's header: AUTHORS is in its repository,
// https://github.com/Kozea/WeasyPrint; the license is validation/LICENSE.)

var (
	fontFaceDescriptors = map[string]fontFaceDescriptorParser{
		"font-family":           fontFamilyDescriptor,
		"src":                   src,
		"font-style":            fontStyleDescriptor,
		"font-weight":           fontWeightDescriptor,
		"font-stretch":          fontStretchDescriptor,
		"font-feature-settings": fontFeatureSettingsDescriptor,
		"font-variant":          fontVariant,
		"unicode-range":         unicodeRange,
	}

	counterStyleDescriptors = map[string]counterStyleDescriptorParser{
		"system":           system,
		"negative":         negative,
		"prefix":           prefix,
		"suffix":           suffix,
		"range":            rangeD,
		"pad":              pad,
		"fallback":         fallback,
		"symbols":          symbols,
		"additive-symbols": additiveSymbols,
	}
)

type NamedProp struct {
	Name  pr.KnownProp
	Value pr.DeclaredValue
}

type FontFaceDescriptors struct {
	Src                 []pr.TaggedString
	FontFamily          pr.String
	FontStyle           pr.String
	FontWeight          pr.IntString
	FontStretch         pr.String
	FontFeatureSettings pr.FontFeatures
	FontVariant         []NamedProp
	UnicodeRange        []pa.UnicodeRange
}

type fontFaceDescriptorParser = func(tokens []Token, baseURL string, out *FontFaceDescriptors) error

// “font-family“ descriptor validation.
// allowSpaces = false
func _fontFamilyDesc(tokens []Token, allowSpaces bool) string {
	allowedsT := utils.Set{string(pa.KIdent): utils.Has}
	if allowSpaces {
		allowedsT.Add(string(pa.KWhitespace))
	}
	if len(tokens) == 1 {
		if str, ok := tokens[0].(pa.String); ok {
			return str.Value
		}
	}

	var values []string
	ok := true
	for _, token := range tokens {
		ok = ok && allowedsT.Has(string(token.Kind()))
		if ident, isToken := token.(pa.Ident); isToken {
			values = append(values, string(ident.Value))
		}
	}
	if len(tokens) > 0 && ok {
		return strings.Join(values, " ")
	}
	return ""
}

func fontFamilyDescriptor(tokens []Token, _ string, out *FontFaceDescriptors) error {
	s := _fontFamilyDesc(tokens, false)
	out.FontFamily = pr.String(s)
	if s == "" {
		return errInvalidValue
	}
	return nil
}

// “src“ descriptor validation.
func _src(tokens []Token, baseURL string) (pr.InnerContent, error) {
	if L := len(tokens); L == 1 || L == 2 {
		token := tokens[len(tokens)-1]
		tokens = tokens[:len(tokens)-1]
		if fn, ok := token.(pa.FunctionBlock); ok && utils.AsciiLower(fn.Name) == "format" {
			token = tokens[len(tokens)-1]
		}
		if fn, ok := token.(pa.FunctionBlock); ok && utils.AsciiLower(fn.Name) == "local" {
			return pr.TaggedString{Tag: pr.Local, S: _fontFamilyDesc(fn.Arguments, true)}, nil
		}
		url, _, err := getUrl(token, baseURL)
		if err != nil {
			return nil, err
		}
		if !url.IsNone() {
			return url, nil
		}
	}
	return nil, nil
}

func src(tokens []Token, baseURL string, out *FontFaceDescriptors) error {
	var l []pr.TaggedString
	for _, part := range pa.SplitOnComma(tokens) {
		result, err := _src(pa.RemoveWhitespace(part), baseURL)
		if err != nil {
			return err
		}
		if result, ok := result.(pr.TaggedString); ok {
			l = append(l, result)
		} else {
			return errInvalidValue
		}
	}
	out.Src = append(out.Src, l...)
	return nil
}

// “font-style“ descriptor validation.
func fontStyleDescriptor(tokens []Token, _ string, out *FontFaceDescriptors) error {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "normal", "italic", "oblique":
		out.FontStyle = pr.String(keyword)
		return nil
	default:
		return fmt.Errorf("unsupported font-style descriptor: %s", keyword)
	}
}

// “font-weight“ descriptor validation.
func fontWeightDescriptor(tokens []Token, _ string, out *FontFaceDescriptors) error {
	if len(tokens) != 1 {
		return errInvalidValue
	}
	token := tokens[0]
	keyword := getKeyword(token)
	if keyword == "normal" || keyword == "bold" {
		out.FontWeight = pr.IntString{String: keyword}
		return nil
	}
	if number, ok := token.(pa.Number); ok && number.IsInt() {
		v := number.Int()
		switch v {
		case 100, 200, 300, 400, 500, 600, 700, 800, 900:
			out.FontWeight = pr.IntString{Int: v}
			return nil
		}
	}
	return errInvalidValue
}

// Validation for the “font-stretch“ descriptor.
func fontStretchDescriptor(tokens []Token, _ string, out *FontFaceDescriptors) error {
	keyword := getSingleKeyword(tokens)
	switch keyword {
	case "ultra-condensed", "extra-condensed", "condensed", "semi-condensed",
		"normal",
		"semi-expanded", "expanded", "extra-expanded", "ultra-expanded":
		out.FontStretch = pr.String(keyword)
		return nil
	default:
		return fmt.Errorf("unsupported font-stretch descriptor: %s", keyword)
	}
}

// “font-feature-settings“ descriptor validation.
func fontFeatureSettingsDescriptor(tokens []Token, _ string, out *FontFaceDescriptors) error {
	s, ok := _fontFeatureSettings(tokens)
	if !ok {
		return errInvalidValue
	}
	out.FontFeatureSettings = s
	return nil
}

// “font-variant“ descriptor validation.
func fontVariant(tokens []Token, _ string, out *FontFaceDescriptors) error {
	if len(tokens) == 1 {
		keyword := getKeyword(tokens[0])
		if keyword == "normal" || keyword == "none" || keyword == "inherit" {
			return nil
		}
	}
	var values []NamedProp
	expanded, err := expandFontVariant(tokens)
	if err != nil {
		return err
	}
	for _, subTokens := range expanded {
		prop, err := validateNonShorthand("", subTokens.name.String(), subTokens.tokens, true)
		if err != nil {
			return errInvalidValue
		}
		values = append(values, NamedProp{prop.name.KnownProp, prop.property})
	}
	out.FontVariant = values
	return nil
}

// “unicode_range“ descriptor validation.
func unicodeRange(tokens []Token, _ string, out *FontFaceDescriptors) error {
	var l []pa.UnicodeRange
	for _, part := range pa.SplitOnComma(tokens) {
		result, err := unicodeRange_(pa.RemoveWhitespace(part))
		if err != nil {
			return err
		}
		if result, ok := result.(pa.UnicodeRange); ok {
			l = append(l, result)
		} else {
			return errInvalidValue
		}
	}
	out.UnicodeRange = append(out.UnicodeRange, l...)
	return nil
}

func unicodeRange_(tokens []Token) (Token, error) {
	if len(tokens) != 1 {
		return nil, errInvalidValue
	}
	token := tokens[0]

	if token.Kind() == pa.KUnicodeRange {
		return token, nil
	}
	return nil, nil
}

// PreprocessFontFaceDescriptors validates the descriptors of an @font-face
// rule (its content), resolving URLs against baseURL. Invalid descriptors
// are ignored, with a warning.
func PreprocessFontFaceDescriptors(baseURL string, descriptors []pa.Compound) FontFaceDescriptors {
	var out FontFaceDescriptors
	preprocessDescriptors(baseURL, descriptors, &out)
	return out
}

// counter-style

type csDescriptors counters.CounterStyleDescriptors

type counterStyleDescriptorParser = func(tokens []Token, baseURL string, out *csDescriptors) error

// “system“ descriptor validation.
func system(tokens []Token, _ string, out *csDescriptors) error {
	if len(tokens) == 0 || len(tokens) > 2 {
		return errInvalidValue
	}

	switch keyword := getKeyword(tokens[0]); keyword {
	case "extends":
		if len(tokens) == 2 {
			secondKeyword := getKeyword(tokens[1])
			if secondKeyword != "" {
				out.System = counters.CounterStyleSystem{Extends: keyword, System: secondKeyword, Number: 0}
				return nil
			}
		}
	case "fixed":
		if len(tokens) == 1 {
			out.System = counters.CounterStyleSystem{Extends: "", System: "fixed", Number: 1}
			return nil
		} else if numb, ok := tokens[1].(pa.Number); ok && numb.IsInt() {
			out.System = counters.CounterStyleSystem{Extends: "", System: "fixed", Number: numb.Int()}
			return nil
		}
	case "cyclic", "numeric", "alphabetic", "symbolic", "additive":
		if len(tokens) == 1 {
			out.System = counters.CounterStyleSystem{Extends: "", System: keyword, Number: 0}
			return nil
		}
	}

	return errInvalidValue
}

// match a String, Ident, or a valid url
func stringIdentOrUrl(token Token, baseURL string) (pr.TaggedString, bool) {
	switch token := token.(type) {
	case pa.String:
		return pr.TaggedString{Tag: pr.String_, S: token.Value}, true
	case pa.Ident:
		return pr.TaggedString{Tag: pr.String_, S: string(token.Value)}, true
	default:
		url, _, _ := getUrl(token, baseURL)
		if !url.IsNone() {
			return pr.TaggedString{Tag: pr.Url, S: url.S}, true
		}
	}
	return pr.TaggedString{}, false
}

// “negative“ descriptor validation.
func negative(tokens []Token, baseURL string, out *csDescriptors) error {
	if len(tokens) > 2 {
		return errInvalidValue
	}

	// <symbol> <symbol>?: the prefix, then the suffix
	var values []pr.TaggedString
	for _, token := range tokens {
		p, ok := stringIdentOrUrl(token, baseURL)
		if !ok {
			return errInvalidValue
		}
		values = append(values, p)
	}

	if len(values) == 1 {
		values = append(values, pr.TaggedString{Tag: pr.String_, S: ""})
	}

	if len(values) == 2 {
		copy(out.Negative[:], values)
		return nil
	}

	return errInvalidValue
}

// @descriptor("counter-style", "prefix", wantsBaseUrl=true)
// @descriptor("counter-style", "suffix", wantsBaseUrl=true)

func prefix(tokens []Token, baseURL string, out *csDescriptors) (err error) {
	out.Prefix, err = _prefixSuffix(tokens, baseURL)
	return err
}

func suffix(tokens []Token, baseURL string, out *csDescriptors) (err error) {
	out.Suffix, err = _prefixSuffix(tokens, baseURL)
	return err
}

// “prefix“ && “suffix“ descriptors validation.
func _prefixSuffix(tokens []Token, baseURL string) (pr.TaggedString, error) {
	if len(tokens) != 1 {
		return pr.TaggedString{}, errInvalidValue
	}
	token := tokens[0]
	if p, ok := stringIdentOrUrl(token, baseURL); ok {
		return p, nil
	}
	return pr.TaggedString{}, errInvalidValue
}

// @descriptor("counter-style")
// @commaSeparatedList
// “range“ descriptor validation.
func rangeD(tokens []Token, _ string, out *csDescriptors) error {
	if len(tokens) == 1 {
		keyword := getSingleKeyword(tokens)
		if keyword == "auto" {
			out.Range = pr.OptionalRanges{Auto: true}
			return nil
		}
	}

	for _, part := range pa.SplitOnComma(tokens) {
		result, err := range_(pa.RemoveWhitespace(part))
		if err != nil {
			return err
		}
		out.Range.Ranges = append(out.Range.Ranges, result)
	}
	return nil
}

func range_(tokens []Token) ([2]int, error) {
	if len(tokens) == 2 {
		var values [2]int
		for i, token := range tokens {
			switch token := token.(type) {
			case pa.Ident:
				if token.Value == "infinite" {
					// Negative infinity as the lower bound, positive
					// infinity as the upper one.
					values[i] = math.MaxInt32
					if i == 0 {
						values[i] = math.MinInt32
					}
					continue
				}
			case pa.Number:
				if token.IsInt() {
					values[i] = token.Int()
					continue
				}
			}
			return [2]int{}, errInvalidValue
		}
		if values[0] <= values[1] {
			return values, nil
		}
	}
	return [2]int{}, errInvalidValue
}

// @descriptor("counter-style", wantsBaseUrl=true)
// “pad“ descriptor validation.
func pad(tokens []Token, baseURL string, out *csDescriptors) error {
	v, err := pad_(tokens, baseURL)
	if err != nil {
		return err
	}
	out.Pad = v
	return nil
}

func pad_(tokens []Token, baseURL string) (out pr.IntNamedString, err error) {
	var hasLength, hasSymbol bool

	if len(tokens) != 2 {
		return out, errInvalidValue
	}

	for _, token := range tokens {
		switch token := token.(type) {
		case pa.Number:
			if token.IsInt() && token.ValueF >= 0 && !hasLength {
				out.Int = token.Int()
				hasLength = true
			}
		default:
			if p, ok := stringIdentOrUrl(token, baseURL); ok {
				out.TaggedString = p
				hasSymbol = true
			}
		}
	}

	if !(hasLength && hasSymbol) {
		return out, errInvalidValue
	}

	return out, nil
}

// @descriptor("counter-style")
// @singleToken
// “fallback“ descriptor validation.
func fallback(tokens []Token, _ string, out *csDescriptors) error {
	if len(tokens) != 1 {
		return errInvalidValue
	}
	token := tokens[0]
	ident := getCustomIdent(token)
	if ident == "none" {
		return errInvalidValue
	}
	out.Fallback = ident
	return nil
}

// @descriptor("counter-style", wantsBaseUrl=true)
// “symbols“ descriptor validation.
func symbols(tokens []Token, baseURL string, out *csDescriptors) error {
	for _, token := range tokens {
		if p, ok := stringIdentOrUrl(token, baseURL); ok {
			out.Symbols = append(out.Symbols, p)
		} else {
			return errInvalidValue
		}
	}
	return nil
}

// @descriptor("counter-style", wantsBaseUrl=true)
// “additive-symbols“ descriptor validation.
func additiveSymbols(tokens []Token, baseURL string, out *csDescriptors) error {
	for _, part := range pa.SplitOnComma(tokens) {
		result, err := pad_(pa.RemoveWhitespace(part), baseURL)
		if err != nil {
			return err
		}
		if L := len(out.AdditiveSymbols); L != 0 && out.AdditiveSymbols[L-1].Int <= result.Int {
			return errInvalidValue
		}
		out.AdditiveSymbols = append(out.AdditiveSymbols, result)
	}
	return nil
}

// PreprocessCounterStyleDescriptors validates the descriptors of a
// @counter-style rule (its content), resolving URLs against baseURL.
// Invalid descriptors are ignored, with a warning.
func PreprocessCounterStyleDescriptors(baseURL string, descriptors []pa.Compound) counters.CounterStyleDescriptors {
	var out counters.CounterStyleDescriptors
	preprocessDescriptors(baseURL, descriptors, (*csDescriptors)(&out))
	return out
}

type parsedDescriptor interface {
	validateDescriptor(baseURL, name string, tokens []Token) error
}

// Default validator for descriptors.
func (d *FontFaceDescriptors) validateDescriptor(baseURL, name string, tokens []Token) error {
	function, ok := fontFaceDescriptors[name]
	if !ok {
		return errors.New("descriptor not supported")
	}

	err := function(tokens, baseURL, d)
	return err
}

func (d *csDescriptors) validateDescriptor(baseURL, name string, tokens []Token) error {
	function, ok := counterStyleDescriptors[name]
	if !ok {
		return errors.New("descriptor not supported")
	}

	err := function(tokens, baseURL, d)
	return err
}

// Filter unsupported names and values for descriptors.
// Log a warning for every ignored descriptor.
func preprocessDescriptors(baseURL string, descriptors []pa.Compound, out parsedDescriptor) {
	for _, descriptor := range descriptors {
		decl, ok := descriptor.(pa.Declaration)
		if !ok || decl.Important {
			continue
		}
		if decl.Name == "font-display" {
			continue
		}

		tokens := pa.RemoveWhitespace(decl.Value)
		name := string(decl.Name)
		err := out.validateDescriptor(baseURL, name, tokens)
		if err != nil {
			logger.L().Warn("Ignored descriptor", "name", name, "value", strings.TrimSpace(pa.Serialize(decl.Value)),
				"line", decl.Pos().Line, "column", decl.Pos().Column, "reason", err)
			continue
		}
	}
}
