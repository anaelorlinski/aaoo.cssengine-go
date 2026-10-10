package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSetSwapDefault(t *testing.T) {
	var buf bytes.Buffer
	installed := slog.New(slog.NewTextHandler(&buf, nil))
	if previous := Swap(installed); previous != nil {
		t.Fatalf("a logger was installed before: %v", previous)
	}
	L().Warn("hello", "k", "v")
	if !strings.Contains(buf.String(), "hello") || !strings.Contains(buf.String(), "k=v") {
		t.Errorf("logged %q", buf.String())
	}
	if previous := Swap(nil); previous != installed {
		t.Error("Swap returns the installed logger")
	}
	// nil: the default logger, resolved at call time
	if L() != slog.Default() {
		t.Error("L() is slog.Default() without an installed logger")
	}
}
