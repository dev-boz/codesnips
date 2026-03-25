package proxy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dev-boz/codesnips/internal/ansi"
	"github.com/dev-boz/codesnips/internal/snippets"
)

func TestDrawBarRestoresCustomScrollRegion(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	tracker := ansi.NewTracker(6, 20, 1, 4)
	tracker.SetScrollRegion(2, 4)
	tracker.SetCursor(3, 5)

	proxy := &instance{
		stdout:         &out,
		rows:           6,
		cols:           20,
		barHeight:      2,
		childRows:      4,
		tracker:        tracker,
		currentSnippet: snippets.Item{Term: "git", Definition: "Distributed version control"},
		currentTheme:   colorThemes[0],
	}

	if err := proxy.drawBar(); err != nil {
		t.Fatalf("drawBar returned error: %v", err)
	}
	if tracker.ScrollTop != 2 || tracker.ScrollBottom != 4 {
		t.Fatalf("drawBar changed tracker scroll region to %d-%d", tracker.ScrollTop, tracker.ScrollBottom)
	}
	if !strings.Contains(out.String(), "\x1b[2;4r") {
		t.Fatalf("drawBar did not restore the custom scroll region: %q", out.String())
	}
}

func TestCleanupIsIdempotent(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	proxy := &instance{
		stdout:    &out,
		rows:      6,
		cols:      20,
		barHeight: 2,
		childRows: 4,
		tracker:   ansi.NewTracker(6, 20, 1, 4),
	}
	proxy.tracker.SetCursor(2, 3)

	proxy.cleanup()
	first := out.String()
	proxy.cleanup()

	if out.String() != first {
		t.Fatalf("cleanup wrote output more than once")
	}
}

func TestFitWidthCountsDisplayColumns(t *testing.T) {
	t.Parallel()

	if got := fitWidth("界", 4); got != "界  " {
		t.Fatalf("fitWidth(%q, 4) = %q, want %q", "界", got, "界  ")
	}
}

func TestWrapTextCountsDisplayColumns(t *testing.T) {
	t.Parallel()

	got := wrapText("界界 x", 4)
	want := []string{"界界", "x   "}

	if len(got) != len(want) {
		t.Fatalf("wrapText returned %d lines, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wrapText line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRenderBarHeaderRemovesExitHint(t *testing.T) {
	t.Parallel()

	proxy := &instance{
		cols:           30,
		barHeight:      1,
		currentSnippet: snippets.Item{Term: "codex", Definition: "CLI agent"},
		currentTheme:   colorThemes[0],
	}

	lines := proxy.renderBar()
	if len(lines) != 1 {
		t.Fatalf("renderBar returned %d lines, want 1", len(lines))
	}
	if strings.Contains(lines[0], "exit to leave proxy") {
		t.Fatalf("renderBar kept the outdated exit hint: %q", lines[0])
	}
}
