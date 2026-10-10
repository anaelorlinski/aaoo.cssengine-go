// Package parser tokenizes and parses CSS, following CSS Syntax Level 3
// (https://www.w3.org/TR/css-syntax-3/): component values ([Tokenize]),
// stylesheets, rule lists, declaration lists and block contents with CSS
// Nesting ([ParseStylesheetBytes], [ParseRuleList],
// [ParseDeclarationList], [ParseBlocksContents]), and serialization back
// to CSS ([Serialize]).
//
// It also parses the CSS Color 4 and 5 <color> syntax ([ParseColor],
// [ParseColorValue]), into the values of package color.
//
// It is a port of tinycss2 (https://github.com/Kozea/tinycss2), whose
// css-parsing-tests corpus it passes, with the deviations documented by
// the tests.
package parser
