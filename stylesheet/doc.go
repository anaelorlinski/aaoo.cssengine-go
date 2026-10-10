// Package stylesheet walks stylesheets for any document language (HTML,
// SVG): it follows @import, evaluates @media and @supports conditions,
// declares cascade layers, and hands the style rules, with their layer, to
// the host (see [Walk]).
package stylesheet

import "github.com/anaelorlinski/aaoo.cssengine-go/parser"

type Token = parser.Token
