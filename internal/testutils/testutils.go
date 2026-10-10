// Package testutils provides the assertion and log-capture helpers used by
// the tests of this module.
package testutils

import (
	"context"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/logger"
)

func AssertEqual(t *testing.T, got, exp any) {
	t.Helper()
	if !reflect.DeepEqual(exp, got) {
		t.Fatalf("expected\n%v\n got \n%v", exp, got)
	}
}

func AssertNoErr(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error %s", err)
	}
}

// CapturedLogs collects the warnings emitted through [logger.L], each
// formatted with [logger.Render].
type CapturedLogs struct {
	mu    sync.Mutex
	stack []string
}

// CaptureLogs installs a logger recording every warning (or higher) emitted
// by the engine, until the end of the test, which restores the previous
// one.
func CaptureLogs(tb testing.TB) *CapturedLogs {
	out := &CapturedLogs{}
	previous := logger.Swap(slog.New(captureHandler{out}))
	tb.Cleanup(func() { logger.Set(previous) })
	return out
}

func (c *CapturedLogs) Logs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.stack...)
}

// CheckEqual compares the captured logs with refs, in order.
func (c *CapturedLogs) CheckEqual(refs []string, t *testing.T) {
	t.Helper()

	gots := c.Logs()
	if len(gots) != len(refs) {
		t.Fatalf("expected %d logs, got %d:\n%s", len(refs), len(gots), strings.Join(gots, "\n"))
	}
	for i, ref := range refs {
		if gots[i] != ref {
			t.Fatalf("expected \n%s\n got \n%s", ref, gots[i])
		}
	}
}

func (c *CapturedLogs) AssertNoLogs(t *testing.T) {
	t.Helper()

	l := c.Logs()
	if len(l) > 0 {
		t.Fatalf("expected no logs, got (%d): \n %s", len(l), strings.Join(l, "\n"))
	}
}

type captureHandler struct{ out *CapturedLogs }

func (h captureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (h captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.stack = append(h.out.stack, logger.Render(r))
	return nil
}

func (h captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h captureHandler) WithGroup(string) slog.Handler      { return h }
