package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dev-boz/codesnips/internal/proxy"
	"github.com/dev-boz/codesnips/internal/snippets"
	"github.com/dev-boz/codesnips/internal/theme"
)

const (
	defaultInterval = 30 * time.Second
	defaultHeight   = 2
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "wrap", "--proxy":
			return runWrap(args[1:])
		case "version", "--version", "-version":
			fmt.Println(buildVersion())
			return 0
		case "--run":
			fmt.Fprintln(os.Stderr, "`snips --run` was removed. Use `snips wrap`.")
			return 2
		}
	}
	return runDefault(args)
}

func runDefault(args []string) int {
	var (
		list         bool
		searchQuery  string
		snippetsFile string
	)

	flags := flag.NewFlagSet("snips", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&list, "list", false, "list terms")
	flags.BoolVar(&list, "l", false, "list terms")
	flags.StringVar(&searchQuery, "search", "", "search snippets")
	flags.StringVar(&searchQuery, "s", "", "search snippets")
	flags.StringVar(&snippetsFile, "file", "", "path to snippets JSON file")
	flags.StringVar(&snippetsFile, "f", "", "path to snippets JSON file")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage()
			return 0
		}
		printUsage()
		return 2
	}

	if flags.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		printUsage()
		return 2
	}

	store, err := snippets.Load(resolveSnippetPath(snippetsFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load snippets: %v\n", err)
		return 1
	}

	switch {
	case list:
		renderList(store.Terms())
		return 0
	case strings.TrimSpace(searchQuery) != "":
		renderSearchResults(strings.TrimSpace(searchQuery), store.Search(searchQuery))
		return 0
	case flags.NArg() == 1:
		key := flags.Arg(0)
		item, ok := store.Get(key)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown term: %s\n", key)
			renderAvailablePreview(store.Terms())
			return 1
		}
		renderPanel(item.Term, item.Definition)
		return 0
	default:
		item := store.Next()
		renderPanel(item.Term, item.Definition)
		return 0
	}
}

func runWrap(args []string) int {
	var (
		barHeight     int
		intervalSecs  int
		snippetsFile  string
		headerStyle   string
		headerReverse bool
	)

	flags := flag.NewFlagSet("snips wrap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.IntVar(&barHeight, "height", defaultHeight, "bar height in terminal rows")
	flags.IntVar(&intervalSecs, "interval", int(defaultInterval.Seconds()), "snippet rotation interval in seconds")
	flags.StringVar(&snippetsFile, "snippets-file", "", "path to snippets JSON file")
	flags.StringVar(&snippetsFile, "file", "", "path to snippets JSON file")
	flags.StringVar(&headerStyle, "header-style", string(proxy.DefaultHeaderStyle), "header style: text or solid (default: solid)")
	flags.BoolVar(&headerReverse, "header-reverse", false, "reverse the header gradient direction")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printWrapUsage()
			return 0
		}
		printWrapUsage()
		return 2
	}

	store, err := snippets.Load(resolveSnippetPath(snippetsFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load snippets: %v\n", err)
		return 1
	}

	resolvedHeaderStyle := proxy.HeaderStyle(strings.ToLower(strings.TrimSpace(headerStyle)))
	if !resolvedHeaderStyle.Valid() {
		fmt.Fprintf(os.Stderr, "invalid --header-style %q (expected: text or solid)\n", headerStyle)
		printWrapUsage()
		return 2
	}

	command := flags.Args()
	if len(command) == 0 {
		command = []string{defaultShell()}
	}

	code, err := proxy.Run(proxy.Config{
		Store:           store,
		Command:         command,
		RequestedHeight: barHeight,
		Interval:        time.Duration(max(1, intervalSecs)) * time.Second,
		HeaderStyle:     resolvedHeaderStyle,
		HeaderReverse:   headerReverse,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "snips wrap failed: %v\n", err)
		return 1
	}
	return code
}

func resolveSnippetPath(provided string) string {
	if strings.TrimSpace(provided) != "" {
		return provided
	}
	localPath := filepath.Join(".", "snippets.json")
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}
	return ""
}

func defaultShell() string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  snips [--version]")
	fmt.Fprintln(os.Stderr, "  snips [--file PATH] [--list | --search QUERY | TERM]")
	fmt.Fprintln(os.Stderr, "  snips wrap [--height N] [--interval SECONDS] [--header-style text|solid] [--header-reverse] [--file PATH] [-- command ...]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  snips --version")
	fmt.Fprintln(os.Stderr, "  snips")
	fmt.Fprintln(os.Stderr, "  snips docker")
	fmt.Fprintln(os.Stderr, "  snips --list")
	fmt.Fprintln(os.Stderr, "  snips --search api")
	fmt.Fprintln(os.Stderr, "  snips wrap -- codex")
	fmt.Fprintln(os.Stderr, "  snips wrap --header-style solid --header-reverse -- codex")
}

func printWrapUsage() {
	fmt.Fprintln(os.Stderr, "Usage: snips wrap [--height N] [--interval SECONDS] [--header-style text|solid] [--header-reverse] [--file PATH] [-- command ...]")
}

func renderPanel(term, definition string) {
	palette := theme.Default()
	titleText := "CodeSnips " + term
	title := theme.RenderLipgloss(titleText, palette.Colors, true)
	body := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(definition)
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.AccentColor(palette.Colors))).
		Padding(0, 1).
		Render(title + "\n" + body)

	fmt.Println()
	fmt.Println(panel)
	fmt.Println()
}

func renderList(terms []string) {
	if len(terms) == 0 {
		fmt.Println()
		fmt.Println("No snippets loaded.")
		fmt.Println()
		return
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69")).Render("Available terms")
	fmt.Println()
	fmt.Println(header)
	for _, term := range terms {
		fmt.Printf("- %s\n", term)
	}
	fmt.Printf("\nTotal: %d terms\n\n", len(terms))
}

func renderSearchResults(query string, results []snippets.SearchResult) {
	if len(results) == 0 {
		fmt.Printf("No matches found for: %s\n", query)
		return
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69")).Render(
		fmt.Sprintf("Found %d match(es)", len(results)),
	)
	fmt.Println()
	fmt.Println(header)

	for i, match := range results {
		if i >= 5 {
			break
		}
		preview := []rune(match.Item.Definition)
		if len(preview) > 100 {
			preview = append(preview[:100], []rune("...")...)
		}
		term := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45")).Render(match.Item.Term)
		desc := lipgloss.NewStyle().Faint(true).Render(string(preview))
		fmt.Printf("- %s: %s\n", term, desc)
	}
	if len(results) > 5 {
		fmt.Printf("- ... and %d more\n", len(results)-5)
	}
	fmt.Println()
}

func renderAvailablePreview(terms []string) {
	if len(terms) == 0 {
		return
	}
	previewCount := min(10, len(terms))
	fmt.Fprintf(os.Stderr, "available terms: %s", strings.Join(terms[:previewCount], ", "))
	if len(terms) > previewCount {
		fmt.Fprint(os.Stderr, ", ...")
	}
	fmt.Fprintln(os.Stderr)
}

func buildVersion() string {
	if strings.TrimSpace(version) == "" {
		return "dev"
	}
	return version
}
