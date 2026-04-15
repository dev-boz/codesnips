package proxy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dev-boz/codesnips/internal/ansi"
	"github.com/dev-boz/codesnips/internal/snippets"
	"github.com/dev-boz/codesnips/internal/theme"
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
		currentTheme:   theme.Default(),
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
		currentTheme:   theme.Default(),
	}

	lines := proxy.renderBar()
	if len(lines) != 1 {
		t.Fatalf("renderBar returned %d lines, want 1", len(lines))
	}
	if strings.Contains(lines[0], "exit to leave proxy") {
		t.Fatalf("renderBar kept the outdated exit hint: %q", lines[0])
	}
}

func TestNextSnippetCyclesThemesInOrder(t *testing.T) {
	t.Parallel()

	store, err := snippets.Load("")
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	proxy := &instance{store: store}
	palettes := theme.Catalog()

	proxy.nextSnippet()
	if proxy.currentTheme.Name != palettes[0].Name {
		t.Fatalf("first theme = %q, want %q", proxy.currentTheme.Name, palettes[0].Name)
	}

	proxy.nextSnippet()
	if proxy.currentTheme.Name != palettes[1].Name {
		t.Fatalf("second theme = %q, want %q", proxy.currentTheme.Name, palettes[1].Name)
	}
}

func TestRenderBarSolidHeaderUsesBackgroundGradient(t *testing.T) {
	t.Parallel()

	proxy := &instance{
		cols:           30,
		barHeight:      2,
		currentSnippet: snippets.Item{Term: "codex", Definition: "CLI agent"},
		currentTheme:   theme.Default(),
		headerStyle:    HeaderStyleSolid,
	}

	lines := proxy.renderBar()
	if len(lines) != 2 {
		t.Fatalf("renderBar returned %d lines, want 2", len(lines))
	}
	if !strings.Contains(lines[0], "48;2;") {
		t.Fatalf("header line did not use a background gradient: %q", lines[0])
	}
	if !strings.Contains(lines[0], "38;2;0;0;0") {
		t.Fatalf("header line did not use black text: %q", lines[0])
	}
	if strings.Contains(lines[1], "48;2;") {
		t.Fatalf("definition line unexpectedly used a background gradient: %q", lines[1])
	}
}

func TestRenderHeaderLineReversesGradientDirection(t *testing.T) {
	t.Parallel()

	got := renderHeaderLine(" CodeSnips ", theme.Default().Colors, HeaderStyleText, true)
	if !strings.Contains(got, " \x1b[38;2;195;103;127mC") {
		t.Fatalf("reversed header did not start from the final palette color: %q", got)
	}
}

func TestDefaultHeaderStyleIsSolid(t *testing.T) {
	t.Parallel()

	if DefaultHeaderStyle != HeaderStyleSolid {
		t.Fatalf("DefaultHeaderStyle = %q, want %q", DefaultHeaderStyle, HeaderStyleSolid)
	}
}
