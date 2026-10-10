// Package utils holds the small helpers shared by the CSS engine packages.
// It is internal: none of these types or functions are part of the public API.
package utils

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"unicode"

	"github.com/anaelorlinski/aaoo.cssengine-go/logger"
)

// Fl is the float type used throughout the engine.
type Fl = float32

var Has = struct{}{}

type Set map[string]struct{}

func (s Set) Add(key string) {
	s[key] = Has
}

func (s Set) Extend(keys []string) {
	for _, key := range keys {
		s[key] = Has
	}
}

func (s Set) Has(key string) bool {
	_, in := s[key]
	return in
}

// Copy returns a deepcopy.
func (s Set) Copy() Set {
	out := make(Set, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

func (s Set) IsNone() bool { return s == nil }

func (s Set) Equal(other Set) bool {
	if len(s) != len(other) {
		return false
	}
	for i := range s {
		if _, in := other[i]; !in {
			return false
		}
	}
	return true
}

func NewSet(values ...string) Set {
	s := make(Set, len(values))
	for _, v := range values {
		s.Add(v)
	}
	return s
}

func Round(x Fl) Fl { return Fl(math.Round(float64(x))) }

func Abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// AsciiLower lowercases the ASCII letters of s, leaving other runes unchanged.
func AsciiLower(s string) string {
	rs := []rune(s)
	out := make([]rune, len(rs))
	for index, c := range rs {
		if c < unicode.MaxASCII {
			c = unicode.ToLower(c)
		}
		out[index] = c
	}
	return string(out)
}

// micmic python urllib.urljoin behavior
func basicUrlJoin(baseURL string, urls *url.URL) (string, error) {
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url : %s", baseURL)
	}
	if urls.Host != "" { // copy the scheme from base
		urls.Scheme = parsedBase.Scheme
		*parsedBase = *urls
	} else if path.IsAbs(urls.Path) { // join from the root
		parsedBase.Path = urls.Path
	} else { // join from the directory
		if path.Ext(parsedBase.Path) != "" {
			parsedBase.Path = path.Join(path.Dir(parsedBase.Path), urls.Path)
		} else {
			parsedBase.Path = path.Join(parsedBase.Path, urls.Path)
		}
	}
	parsedBase.RawQuery = urls.RawQuery
	parsedBase.RawFragment = urls.RawFragment
	parsedBase.Fragment = urls.Fragment
	return parsedBase.String(), nil
}

// SafeUrljoin joins urls to baseURL (allowRelative is false by default).
func SafeUrljoin(baseURL, urls string, allowRelative bool) (string, error) {
	parsed, err := url.Parse(urls)
	if err != nil {
		return "", fmt.Errorf("invalid url : %s (%s)", urls, err)
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	} else if baseURL != "" {
		return basicUrlJoin(baseURL, parsed)
	} else if allowRelative {
		return parsed.String(), nil
	} else {
		return "", errors.New("Relative URI reference without a base URI: " + urls)
	}
}

func Unquote(s string) string {
	unescaped, err := url.PathUnescape(s)
	if err != nil {
		logger.L().Warn("invalid escape in URL", "value", s, "err", err)
		return ""
	}
	return unescaped
}

// RoundPrec rounds f with n digits precision
func RoundPrec(f Fl, n int) Fl {
	n10 := math.Pow10(n)
	return Fl(math.Round(float64(f)*n10) / n10)
}

// Round6 rounds f with 6 digits precision
func Round6(f Fl) Fl { return RoundPrec(f, 6) }
