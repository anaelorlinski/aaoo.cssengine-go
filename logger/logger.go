// Package logger holds the *slog.Logger the CSS engine reports through:
// ignored or invalid declarations, unexpected units, and the like.
//
// By default records go to [slog.Default], resolved at call time, so an
// application controls them with [slog.SetDefault]. [Set] installs a
// dedicated logger instead.
package logger

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
)

var current atomic.Pointer[slog.Logger]

// Set installs the logger used by the CSS engine. nil restores the default.
// It is safe to call concurrently with rendering.
func Set(l *slog.Logger) { current.Store(l) }

// Swap is [Set], returning the logger installed before (nil for the
// default), to restore it later.
func Swap(l *slog.Logger) (previous *slog.Logger) { return current.Swap(l) }

// L returns the installed logger, or [slog.Default] at call time.
func L() *slog.Logger {
	if l := current.Load(); l != nil {
		return l
	}
	return slog.Default()
}

// Render formats a record as a single line: the message followed by
// "key=value" pairs, values unquoted. It is meant for plain-text sinks and
// for tests asserting on warnings.
func Render(r slog.Record) string {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Resolve().Any())
		return true
	})
	return b.String()
}
