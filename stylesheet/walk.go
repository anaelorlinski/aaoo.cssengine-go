package stylesheet

import (
	"errors"
	"fmt"
	"slices"

	"github.com/anaelorlinski/aaoo.cssengine-go/cascade"
	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
	"github.com/anaelorlinski/aaoo.cssengine-go/logger"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
)

// Fetcher fetches the stylesheet at url (absolute), returning its content
// and its URL after redirections, against which its own relative URLs
// resolve.
type Fetcher func(url string) (content []byte, resolvedURL string, err error)

// Options configure [Walk].
type Options struct {
	// Device is the medium @media rules and @import media conditions are
	// evaluated against.
	Device Device
	// Fetch fetches the stylesheets of @import rules; nil ignores them,
	// with a warning.
	Fetch Fetcher
	// Layers receives the cascade layers the stylesheet declares; the
	// layer passed to [Handler] is an id of it. Required.
	Layers *cascade.Layers
	// MaxImportDepth and MaxImports bound @import; zero means
	// [limits.ImportDepth] and [limits.Imports].
	MaxImportDepth, MaxImports int
}

// Handler receives what [Walk] does not interpret itself.
type Handler interface {
	// StyleRule is called for each style rule, in order, with the cascade
	// layer it belongs to and the URL of its stylesheet. It reports
	// whether the rule is valid: a valid rule ends the place for @import.
	StyleRule(rule parser.QualifiedRule, layer int, baseURL string) bool
	// AtRule is called for the at-rules other than @import, @media,
	// @supports, @layer and @charset (@page, @font-face, ... in HTML). It
	// reports whether it handled the rule; unhandled rules are reported as
	// unknown.
	AtRule(rule parser.AtRule, layer int, baseURL string) bool
}

// Walk goes through the rules of a stylesheet located at baseURL, in
// order, as a document language using CSS does:
//
//   - @import rules, only before the other rules (except @charset and
//     @layer statements), fetch and walk their stylesheet, under their
//     media and supports() conditions, into their layer (layer, layer());
//     cycles and the limits of Options end the import chain;
//   - @media and @supports rules walk their content when their condition
//     holds;
//   - @layer blocks and statements declare cascade layers
//     (https://www.w3.org/TR/css-cascade-5/#layering); layers in false
//     conditions are not declared;
//   - style rules and the other at-rules go to h.
//
// Problems are reported as warnings, through the module's logger, and the
// offending rule is ignored.
func Walk(rules []parser.Compound, baseURL string, opts Options, h Handler) Report {
	if opts.MaxImportDepth == 0 {
		opts.MaxImportDepth = limits.ImportDepth
	}
	if opts.MaxImports == 0 {
		opts.MaxImports = limits.Imports
	}
	w := walker{opts: opts, handler: h}
	if baseURL != "" {
		w.chain = append(w.chain, baseURL)
	}
	w.walk(rules, baseURL, 0, true)
	return w.report
}

// Report tells what the result of a [Walk] depends on.
type Report struct {
	// SizeDependent is true when a media condition evaluated by the walk
	// tests the size of the device: walking again with another device size
	// may give other rules.
	SizeDependent bool
	// SizeQueries are those media query lists, in the order evaluated. A
	// walk with another device for which each of them evaluates the same
	// gives the same rules (with the same fetched content).
	SizeQueries [][]MediaQuery
}

type walker struct {
	opts    Options
	handler Handler
	chain   []string // the stylesheets being imported, outermost first
	fetched int      // number of stylesheets imported so far
	report  Report
}

// matchesMedia evaluates a media query list, recording size dependence.
func (w *walker) matchesMedia(queryList []MediaQuery) bool {
	if DependsOnSize(queryList) {
		w.report.SizeDependent = true
		w.report.SizeQueries = append(w.report.SizeQueries, queryList)
	}
	return EvaluateMediaQuery(queryList, w.opts.Device)
}

func (w *walker) walk(rules []parser.Compound, baseURL string, layer int, importsAllowed bool) {
	sawImport := false
	for _, rule := range rules {
		switch rule := rule.(type) {
		case parser.ParseError:
			logger.L().Warn("Parse error", "line", rule.Pos().Line, "column", rule.Pos().Column, "reason", rule.Message)
		case parser.QualifiedRule:
			if w.handler.StyleRule(rule, layer, baseURL) {
				importsAllowed = false
			}
		case parser.AtRule:
			switch utils.AsciiLower(rule.AtKeyword) {
			case "charset":
				// the encoding is resolved when fetching
			case "import":
				if !importsAllowed {
					logger.L().Warn("@import rule not at the beginning of the stylesheet was ignored", "rule", parser.Serialize(rule.Prelude))
					continue
				}
				sawImport = true
				w.importRule(rule, baseURL, layer)
			case "media":
				importsAllowed = false
				if rule.Content == nil || !w.matchesMedia(ParseMediaQuery(rule.Prelude)) {
					continue
				}
				w.walk(parser.ParseRuleList(rule.Content, false, false), baseURL, layer, false)
			case "supports":
				importsAllowed = false
				if rule.Content == nil || !EvaluateSupports(rule.Prelude) {
					continue
				}
				w.walk(parser.ParseRuleList(rule.Content, false, false), baseURL, layer, false)
			case "layer":
				layerRule, ok := cascade.ParseLayerRule(rule.Prelude, rule.Content != nil)
				if !ok {
					logger.L().Warn("Invalid @layer rule, ignored", "rule", parser.Serialize(rule.Prelude),
						"line", rule.Pos().Line, "column", rule.Pos().Column)
					continue
				}
				blockLayer := layerRule.Declare(w.opts.Layers, layer)
				if !layerRule.Block {
					// a statement may precede the @import rules, not come
					// between them (CSS Cascade 5 §2.1)
					if sawImport {
						importsAllowed = false
					}
					continue
				}
				importsAllowed = false
				w.walk(parser.ParseRuleList(rule.Content, false, false), baseURL, blockLayer, false)
			default:
				if w.handler.AtRule(rule, layer, baseURL) {
					importsAllowed = false
				} else {
					logger.L().Warn("Unknown rule", "rule", "@"+rule.AtKeyword+" "+parser.Serialize(rule.Prelude),
						"line", rule.Pos().Line, "column", rule.Pos().Column)
				}
			}
		}
	}
}

// importRule handles an @import rule:
// @import [ <url> | <string> ] [ layer | layer(<name>) ]? [ supports(...) ]? <media-query-list>?
func (w *walker) importRule(rule parser.AtRule, baseURL string, layer int) {
	if rule.Content != nil {
		logger.L().Warn("@import with a block was ignored", "rule", parser.Serialize(rule.Prelude))
		return
	}
	tokens := parser.RemoveWhitespace(rule.Prelude)
	url, ok := importURL(tokens)
	if !ok {
		logger.L().Warn("Invalid @import, ignored", "rule", parser.Serialize(rule.Prelude))
		return
	}
	layerRule, hasLayer, conditions, ok := cascade.ParseImportLayer(tokens[1:])
	if !ok {
		logger.L().Warn("Invalid layer name in @import, ignored", "rule", parser.Serialize(rule.Prelude))
		return
	}
	if len(conditions) > 0 {
		if fn, ok := conditions[0].(parser.FunctionBlock); ok && utils.AsciiLower(fn.Name) == "supports" {
			if !evaluateSupportsFunction(fn.Arguments) {
				return
			}
			conditions = conditions[1:]
		}
	}
	if !w.matchesMedia(ParseMediaQuery(conditions)) {
		return
	}
	// the layer is declared even if the import fails
	importLayer := layer
	if hasLayer {
		importLayer = layerRule.Declare(w.opts.Layers, layer)
	}

	if w.opts.Fetch == nil {
		logger.L().Warn("No URL fetcher, @import ignored", "url", url)
		return
	}
	joined, err := utils.SafeUrljoin(baseURL, url, false)
	if err != nil {
		logger.L().Warn("Ignored @import", "url", url, "reason", err)
		return
	}
	if err := w.enter(joined); err != nil {
		logger.L().Warn("Ignored @import", "url", joined, "reason", err)
		return
	}
	defer w.leave()
	content, resolvedURL, err := w.opts.Fetch(joined)
	if err != nil {
		logger.L().Warn("Failed to load stylesheet", "url", joined, "reason", err)
		return
	}
	if resolvedURL == "" {
		resolvedURL = joined
	}
	w.walk(parser.ParseStylesheetBytes(content, false, false), resolvedURL, importLayer, true)
}

// importURL reads the <url> or <string> starting an @import prelude
// (without whitespace), including url("...") with a quoted argument, which
// the tokenizer gives as a function.
func importURL(tokens []parser.Token) (string, bool) {
	if len(tokens) == 0 {
		return "", false
	}
	switch token := tokens[0].(type) {
	case parser.String:
		return token.Value, true
	case parser.URL:
		return token.Value, true
	case parser.FunctionBlock:
		args := parser.RemoveWhitespace(token.Arguments)
		if utils.AsciiLower(token.Name) != "url" || len(args) != 1 {
			return "", false
		}
		if str, ok := args[0].(parser.String); ok {
			return str.Value, true
		}
	}
	return "", false
}

// evaluateSupportsFunction evaluates the supports() condition of an
// @import: a <supports-condition> or a <declaration>, the latter as if in
// parentheses.
func evaluateSupportsFunction(args []Token) bool {
	var block parser.ParenthesesBlock
	block.Arguments = args
	if ok, valid := supportsInParens([]Token{block}, 0); valid {
		return ok
	}
	logger.L().Warn("Invalid supports() condition in @import", "condition", parser.Serialize(args))
	return false
}

func (w *walker) enter(url string) error {
	if slices.Contains(w.chain, url) {
		return errors.New("@import cycle")
	}
	if len(w.chain) > w.opts.MaxImportDepth {
		return fmt.Errorf("more than %d nested @import", w.opts.MaxImportDepth)
	}
	if w.fetched >= w.opts.MaxImports {
		return fmt.Errorf("more than %d @import", w.opts.MaxImports)
	}
	w.fetched++
	w.chain = append(w.chain, url)
	return nil
}

func (w *walker) leave() { w.chain = w.chain[:len(w.chain)-1] }
