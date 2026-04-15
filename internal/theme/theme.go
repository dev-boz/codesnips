package theme

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type RGB struct {
	R uint8
	G uint8
	B uint8
}

type Palette struct {
	Name   string
	Colors []RGB
}

var catalog = mustBuildCatalog([]struct {
	name   string
	colors []string
}{
	{name: "Default Dark/Light", colors: []string{"#4796E4", "#847ACE", "#C3677F"}},
	{name: "Ayu Dark", colors: []string{"#FFB454", "#FFFFFF", "#F26D78"}},
	{name: "Dracula", colors: []string{"#FF79C6", "#FFFFFF", "#8BE9FD"}},
	{name: "GitHub Dark", colors: []string{"#79B8FF", "#FFFFFF", "#85E89D"}},
	{name: "Atom One Dark", colors: []string{"#61AEEE", "#FFFFFF", "#98C379"}},
	{name: "Shades of Purple", colors: []string{"#8C75FF", "#B39DF1", "#FF7AA2"}},
	{name: "Solarized Dark", colors: []string{"#268BD2", "#FFFFFF", "#2AA198"}},
	{name: "Tokyo Night", colors: []string{"#7AA2F7", "#BB9AF7", "#7DCFFF"}},
	{name: "ANSI Dark", colors: []string{"cyan", "#FFFFFF", "green"}},
	{name: "Holiday Dark", colors: []string{"#FF6B6B", "#FFFFFF", "#5ECF78"}},
	{name: "Ayu Light", colors: []string{"#399EE6", "#FFFFFF", "#86B300"}},
	{name: "GitHub Light", colors: []string{"#6F86B6", "#FFFFFF", "#2FA7A7"}},
	{name: "GoogleCode", colors: []string{"#3AA8A8", "#FFFFFF", "#B06BCF"}},
	{name: "XCode", colors: []string{"#6B5BFF", "#FFFFFF", "#49A95B"}},
	{name: "Solarized Light", colors: []string{"#268BD2", "#FFFFFF", "#2AA198"}},
	{name: "ANSI Light", colors: []string{"#6B7CFF", "#FFFFFF", "#59D46C"}},
	{name: "Nord", colors: []string{"#88C0D0", "#81A1C1", "#B48EAD"}},
	{name: "Catppuccin Mocha", colors: []string{"#89B4FA", "#CBA6F7", "#F5C2E7"}},
	{name: "Gruvbox Dark", colors: []string{"#FABD2F", "#FE8019", "#FB4934"}},
	{name: "Monokai", colors: []string{"#78DCE8", "#A9DC76", "#FF6188"}},
})

var namedColors = map[string]RGB{
	"blue":  {R: 0x00, G: 0x00, B: 0xFF},
	"cyan":  {R: 0x00, G: 0xFF, B: 0xFF},
	"green": {R: 0x00, G: 0xFF, B: 0x00},
}

func Catalog() []Palette {
	palettes := make([]Palette, len(catalog))
	copy(palettes, catalog)
	return palettes
}

func Default() Palette {
	return catalog[0]
}

func RenderANSI(text string, colors []RGB) string {
	if len(text) == 0 || len(colors) == 0 {
		return text
	}

	var result strings.Builder
	leading, content, trailing := splitOuterSpaces(text)
	result.WriteString(leading)
	runes := []rune(content)
	if len(runes) == 0 {
		result.WriteString(trailing)
		return result.String()
	}
	for i, r := range runes {
		color := gradientColor(colors, i, len(runes))
		result.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s", color.R, color.G, color.B, string(r)))
	}
	result.WriteString("\x1b[0m")
	result.WriteString(trailing)
	return result.String()
}

func RenderLipgloss(text string, colors []RGB, bold bool) string {
	if len(text) == 0 || len(colors) == 0 {
		return text
	}

	var result strings.Builder
	leading, content, trailing := splitOuterSpaces(text)
	result.WriteString(leading)
	runes := []rune(content)
	if len(runes) == 0 {
		result.WriteString(trailing)
		return result.String()
	}
	for i, r := range runes {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(gradientColor(colors, i, len(runes)).Hex()))
		if bold {
			style = style.Bold(true)
		}
		result.WriteString(style.Render(string(r)))
	}
	result.WriteString(trailing)
	return result.String()
}

func RenderANSIBackground(text string, colors []RGB, foreground RGB) string {
	if len(text) == 0 || len(colors) == 0 {
		return text
	}

	var result strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		color := gradientColor(colors, i, len(runes))
		result.WriteString(
			fmt.Sprintf(
				"\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s",
				foreground.R,
				foreground.G,
				foreground.B,
				color.R,
				color.G,
				color.B,
				string(r),
			),
		)
	}
	result.WriteString("\x1b[0m")
	return result.String()
}

func Reverse(colors []RGB) []RGB {
	reversed := make([]RGB, len(colors))
	for i := range colors {
		reversed[len(colors)-1-i] = colors[i]
	}
	return reversed
}

func AccentColor(colors []RGB) string {
	if len(colors) == 0 {
		return "#847ACE"
	}
	return gradientColor(colors, 1, 3).Hex()
}

func mustBuildCatalog(specs []struct {
	name   string
	colors []string
}) []Palette {
	palettes := make([]Palette, 0, len(specs))
	for _, spec := range specs {
		palette := Palette{
			Name:   spec.name,
			Colors: make([]RGB, 0, len(spec.colors)),
		}
		for _, value := range spec.colors {
			palette.Colors = append(palette.Colors, mustParseColor(value))
		}
		palettes = append(palettes, palette)
	}
	return palettes
}

func mustParseColor(value string) RGB {
	color, err := parseColor(value)
	if err != nil {
		panic(err)
	}
	return color
}

func parseColor(value string) (RGB, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if named, ok := namedColors[normalized]; ok {
		return named, nil
	}
	if !strings.HasPrefix(normalized, "#") {
		return RGB{}, fmt.Errorf("unsupported colour %q", value)
	}

	hex := strings.TrimPrefix(normalized, "#")
	switch len(hex) {
	case 3:
		hex = strings.Repeat(string(hex[0]), 2) +
			strings.Repeat(string(hex[1]), 2) +
			strings.Repeat(string(hex[2]), 2)
	case 6:
	default:
		return RGB{}, fmt.Errorf("invalid hex colour %q", value)
	}

	r, err := strconv.ParseUint(hex[0:2], 16, 8)
	if err != nil {
		return RGB{}, fmt.Errorf("invalid red channel in %q: %w", value, err)
	}
	g, err := strconv.ParseUint(hex[2:4], 16, 8)
	if err != nil {
		return RGB{}, fmt.Errorf("invalid green channel in %q: %w", value, err)
	}
	b, err := strconv.ParseUint(hex[4:6], 16, 8)
	if err != nil {
		return RGB{}, fmt.Errorf("invalid blue channel in %q: %w", value, err)
	}

	return RGB{R: uint8(r), G: uint8(g), B: uint8(b)}, nil
}

func gradientColor(colors []RGB, index, total int) RGB {
	if len(colors) == 0 {
		return RGB{}
	}
	if len(colors) == 1 || total <= 1 {
		return colors[0]
	}

	position := float64(index) / float64(total-1)
	segmentWidth := 1.0 / float64(len(colors)-1)
	segment := int(position / segmentWidth)
	if segment >= len(colors)-1 {
		return colors[len(colors)-1]
	}

	start := colors[segment]
	end := colors[segment+1]
	segmentStart := float64(segment) * segmentWidth
	progress := (position - segmentStart) / segmentWidth

	return RGB{
		R: lerp(start.R, end.R, progress),
		G: lerp(start.G, end.G, progress),
		B: lerp(start.B, end.B, progress),
	}
}

func lerp(start, end uint8, progress float64) uint8 {
	value := float64(start) + (float64(end)-float64(start))*progress
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}
	return uint8(value + 0.5)
}

func (c RGB) Hex() string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

func Black() RGB {
	return RGB{}
}

func splitOuterSpaces(text string) (leading, content, trailing string) {
	start := 0
	for start < len(text) && text[start] == ' ' {
		start++
	}

	end := len(text)
	for end > start && text[end-1] == ' ' {
		end--
	}

	return text[:start], text[start:end], text[end:]
}
