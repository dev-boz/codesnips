package theme

import (
	"strings"
	"testing"
)

func TestParseColorSupportsShorthandAndNamedColours(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  RGB
	}{
		{input: "#458", want: RGB{R: 0x44, G: 0x55, B: 0x88}},
		{input: "#066", want: RGB{R: 0x00, G: 0x66, B: 0x66}},
		{input: "cyan", want: RGB{R: 0x00, G: 0xFF, B: 0xFF}},
	}

	for _, test := range tests {
		got, err := parseColor(test.input)
		if err != nil {
			t.Fatalf("parseColor(%q) returned error: %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("parseColor(%q) = %#v, want %#v", test.input, got, test.want)
		}
	}
}

func TestRenderANSIUsesTruecolorGradient(t *testing.T) {
	t.Parallel()

	got := RenderANSI("ABC", []RGB{
		{R: 0x47, G: 0x96, B: 0xE4},
		{R: 0x84, G: 0x7A, B: 0xCE},
		{R: 0xC3, G: 0x67, B: 0x7F},
	})

	if !strings.Contains(got, "\x1b[38;2;71;150;228mA") {
		t.Fatalf("RenderANSI() did not emit the starting truecolor sequence: %q", got)
	}
	if !strings.Contains(got, "\x1b[38;2;132;122;206mB") {
		t.Fatalf("RenderANSI() did not emit the midpoint truecolor sequence: %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("RenderANSI() did not reset styles: %q", got)
	}
}

func TestRenderANSIIgnoresOuterPaddingForGradientLength(t *testing.T) {
	t.Parallel()

	got := RenderANSI("  ABC   ", []RGB{
		{R: 0x47, G: 0x96, B: 0xE4},
		{R: 0x84, G: 0x7A, B: 0xCE},
		{R: 0xC3, G: 0x67, B: 0x7F},
	})

	if !strings.HasPrefix(got, "  \x1b[38;2;71;150;228mA") {
		t.Fatalf("RenderANSI() colored leading padding or used padded width for the gradient: %q", got)
	}
	if !strings.Contains(got, "\x1b[38;2;132;122;206mB") {
		t.Fatalf("RenderANSI() did not keep the midpoint based on visible content length: %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[0m   ") {
		t.Fatalf("RenderANSI() colored trailing padding: %q", got)
	}
}

func TestAccentColorUsesGradientMidpoint(t *testing.T) {
	t.Parallel()

	got := AccentColor([]RGB{
		{R: 0x47, G: 0x96, B: 0xE4},
		{R: 0x84, G: 0x7A, B: 0xCE},
		{R: 0xC3, G: 0x67, B: 0x7F},
	})

	if got != "#847ACE" {
		t.Fatalf("AccentColor() = %q, want %q", got, "#847ACE")
	}
}

func TestCatalogPalettesUseAtLeastThreeStops(t *testing.T) {
	t.Parallel()

	for _, palette := range Catalog() {
		if len(palette.Colors) < 3 {
			t.Fatalf("palette %q has %d colors, want at least 3", palette.Name, len(palette.Colors))
		}
	}
}

func TestReverseReturnsReversedCopy(t *testing.T) {
	t.Parallel()

	colors := []RGB{{R: 1}, {R: 2}, {R: 3}}
	got := Reverse(colors)

	if got[0].R != 3 || got[1].R != 2 || got[2].R != 1 {
		t.Fatalf("Reverse() = %#v, want reversed order", got)
	}
	if colors[0].R != 1 || colors[2].R != 3 {
		t.Fatalf("Reverse() mutated the input slice: %#v", colors)
	}
}
