package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dev-boz/codesnips/internal/ansi"
	"github.com/dev-boz/codesnips/internal/pty"
	"github.com/dev-boz/codesnips/internal/snippets"
)

var colorThemes = []colorTheme{
	{headerBG: "48;5;25", headerFG: "38;5;231", bodyBG: "48;5;159", bodyFG: "38;5;16"},
	{headerBG: "48;5;88", headerFG: "38;5;230", bodyBG: "48;5;223", bodyFG: "38;5;16"},
	{headerBG: "48;5;22", headerFG: "38;5;230", bodyBG: "48;5;157", bodyFG: "38;5;16"},
	{headerBG: "48;5;60", headerFG: "38;5;231", bodyBG: "48;5;189", bodyFG: "38;5;16"},
	{headerBG: "48;5;130", headerFG: "38;5;230", bodyBG: "48;5;223", bodyFG: "38;5;16"},
	{headerBG: "48;5;24", headerFG: "38;5;230", bodyBG: "48;5;117", bodyFG: "38;5;16"},
}

type Config struct {
	Store           *snippets.Store
	Command         []string
	RequestedHeight int
	Interval        time.Duration
}

type colorTheme struct {
	headerBG string
	headerFG string
	bodyBG   string
	bodyFG   string
}

type exitStatus struct {
	status syscall.WaitStatus
	err    error
}

type instance struct {
	masterFile      *os.File
	childPID        int
	stdinFD         int
	stdout          io.Writer
	requestedHeight int
	barHeight       int
	rows            int
	cols            int
	childTop        int
	childRows       int
	store           *snippets.Store
	currentSnippet  snippets.Item
	currentTheme    colorTheme
	altScreenActive bool
	altSavedRow     int
	altSavedCol     int
	tracker         *ansi.Tracker
	rewriter        *ansi.Rewriter
}

func Run(config Config) (int, error) {
	if config.Store == nil {
		return 1, errors.New("snippet store is required")
	}
	if len(config.Command) == 0 {
		return 1, errors.New("command is required")
	}
	if config.Interval <= 0 {
		config.Interval = 30 * time.Second
	}

	if !pty.IsTTY(os.Stdin.Fd()) || !pty.IsTTY(os.Stdout.Fd()) {
		return 2, errors.New("snips wrap requires a real TTY on stdin/stdout")
	}

	rows, cols, err := pty.TerminalSize(int(os.Stdin.Fd()))
	if err != nil {
		return 1, fmt.Errorf("failed to read terminal size: %w", err)
	}

	effectiveBar := min(max(0, config.RequestedHeight), max(0, rows-1))
	childRows := max(1, rows-effectiveBar)
	child, err := pty.Start(config.Command, childRows, cols)
	if err != nil {
		return 1, fmt.Errorf("failed to start child PTY: %w", err)
	}
	defer child.Master.Close()

	mode, err := pty.EnableRawMode(int(os.Stdin.Fd()))
	if err != nil {
		return 1, fmt.Errorf("failed to switch terminal to raw mode: %w", err)
	}
	defer mode.Restore()

	proxy := newInstance(child.Master, child.PID, int(os.Stdin.Fd()), os.Stdout, config.RequestedHeight, config.Store)
	proxy.updateLayout(rows, cols)
	proxy.nextSnippet()
	if err := proxy.initializeTerminal(); err != nil {
		return 1, fmt.Errorf("failed to initialize proxy terminal: %w", err)
	}
	defer proxy.cleanup()

	outputCh := make(chan []byte, 32)
	exitCh := make(chan exitStatus, 1)
	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)
	defer signal.Stop(resizeCh)

	go readPTY(child.Master, outputCh)
	go waitChild(child.PID, exitCh)
	go copyInput(child.Master, os.Stdin)

	rotateTicker := time.NewTicker(config.Interval)
	defer rotateTicker.Stop()

	var (
		readerClosed bool
		childExited  bool
		childStatus  exitStatus
		loopErr      error
	)

loop:
	for {
		select {
		case chunk, ok := <-outputCh:
			if !ok {
				readerClosed = true
				if childExited {
					break loop
				}
				continue
			}
			if err := proxy.handleChildOutput(chunk); err != nil {
				loopErr = err
				break loop
			}
		case <-rotateTicker.C:
			proxy.nextSnippet()
			if err := proxy.drawBar(); err != nil {
				loopErr = err
				break loop
			}
		case <-resizeCh:
			if err := proxy.resize(); err != nil {
				loopErr = err
				break loop
			}
		case status := <-exitCh:
			childExited = true
			childStatus = status
			if readerClosed {
				break loop
			}
		}
	}

	pty.SignalGroup(child.PID, syscall.SIGHUP)

	// Explicitly clean up before return from this function.
	proxy.cleanup()
	mode.Restore()

	if !childExited {
		childStatus = <-exitCh
		childExited = true
	}

	if childStatus.err != nil {
		return 1, fmt.Errorf("child wait failed: %w", childStatus.err)
	}
	if childStatus.status.Exited() {
		if loopErr != nil {
			return childStatus.status.ExitStatus(), loopErr
		}
		return childStatus.status.ExitStatus(), nil
	}
	if childStatus.status.Signaled() {
		return 128 + int(childStatus.status.Signal()), nil
	}
	if loopErr != nil {
		return 1, loopErr
	}
	return 0, nil
}

func newInstance(master *os.File, childPID int, stdinFD int, stdout io.Writer, requestedHeight int, store *snippets.Store) *instance {
	return &instance{
		masterFile:      master,
		childPID:        childPID,
		stdinFD:         stdinFD,
		stdout:          stdout,
		requestedHeight: max(0, requestedHeight),
		store:           store,
	}
}

func (p *instance) updateLayout(rows, cols int) {
	p.rows = max(1, rows)
	p.cols = max(20, cols)
	maxBar := max(0, p.rows-1)
	p.barHeight = min(p.requestedHeight, maxBar)
	p.childTop = 1
	p.childRows = max(1, p.rows-p.barHeight)
	if p.tracker == nil {
		p.tracker = ansi.NewTracker(p.rows, p.cols, 1, p.childRows)
	} else {
		p.tracker.Resize(p.rows, p.cols, 1, p.childRows)
	}

	layout := ansi.Layout{
		ChildTop:  p.childTop,
		ChildRows: p.childRows,
		Cols:      p.cols,
	}

	if p.rewriter == nil {
		p.rewriter = ansi.NewRewriter(p.tracker, layout, ansi.Callbacks{
			SaveAltCursor:      p.saveAltCursor,
			RestoreAltCursor:   p.restoreAltCursor,
			SetAltScreenActive: p.setAltScreenActive,
		})
		return
	}
	p.rewriter.UpdateLayout(layout)
}

func (p *instance) setAltScreenActive(active bool) {
	p.altScreenActive = active
}

func (p *instance) nextSnippet() {
	p.currentSnippet = p.store.Next()
	p.currentTheme = colorThemes[p.store.Intn(len(colorThemes))]
}

func (p *instance) saveAltCursor() {
	p.altSavedRow = p.tracker.Row
	p.altSavedCol = p.tracker.Col
}

func (p *instance) restoreAltCursor() {
	if p.altSavedRow == 0 {
		p.tracker.SetCursor(p.childTop, 1)
		return
	}
	p.tracker.SetCursor(p.altSavedRow, p.altSavedCol)
}

func (p *instance) initializeTerminal() error {
	if err := p.applyChildViewport(); err != nil {
		return err
	}
	return p.drawBar()
}

func (p *instance) applyChildViewport() error {
	seq := bytes.NewBuffer(nil)
	seq.WriteString("\x1b[0m")
	seq.WriteString("\x1b[r")
	if p.barHeight > 0 {
		seq.WriteString(fmt.Sprintf("\x1b[1;%dr", p.childRows))
	}
	seq.WriteString(ansi.FormatCUP(1, 1))
	_, err := p.stdout.Write(seq.Bytes())
	return err
}

func (p *instance) drawBar() error {
	if p.currentSnippet.Term == "" {
		p.nextSnippet()
	}

	lines := p.renderBar()

	buf := bytes.NewBuffer(nil)
	buf.WriteString("\x1b[0m")
	buf.WriteString("\x1b[r")
	for i := 0; i < p.barHeight; i++ {
		row := p.childRows + 1 + i
		buf.WriteString(ansi.FormatCUP(row, 1))
		buf.WriteString("\x1b[2K")
		if i < len(lines) {
			buf.WriteString(lines[i])
		}
		buf.WriteString("\x1b[0m")
	}
	if p.barHeight > 0 {
		buf.WriteString(fmt.Sprintf("\x1b[1;%dr", p.childRows))
	}
	buf.WriteString(ansi.FormatCUP(p.tracker.Row, p.tracker.Col))
	_, err := p.stdout.Write(buf.Bytes())
	return err
}

func (p *instance) renderBar() []string {
	if p.barHeight == 0 {
		return nil
	}

	defWidth := max(1, p.cols-4)
	header := fitWidth(
		fmt.Sprintf(
			" CodeSnips | %s | Ctrl+C to child | exit to leave proxy ",
			strings.ToUpper(p.currentSnippet.Term),
		),
		p.cols,
	)
	definitionLines := wrapText(p.currentSnippet.Definition, defWidth)
	bodyRows := max(0, p.barHeight-1)
	lines := make([]string, 0, p.barHeight)

	lines = append(lines, colorLine(header, p.currentTheme.headerBG, p.currentTheme.headerFG))
	for i := 0; i < bodyRows; i++ {
		text := ""
		if i < len(definitionLines) {
			text = "  " + definitionLines[i]
		}
		lines = append(lines, colorLine(fitWidth(text, p.cols), p.currentTheme.bodyBG, p.currentTheme.bodyFG))
	}
	return lines
}

func (p *instance) handleChildOutput(data []byte) error {
	out := p.rewriter.Feed(data)
	if len(out) == 0 {
		return nil
	}
	_, err := p.stdout.Write(out)
	return err
}

func (p *instance) resize() error {
	rows, cols, err := pty.TerminalSize(p.stdinFD)
	if err != nil {
		return err
	}
	p.updateLayout(rows, cols)
	if err := pty.SetSize(p.masterFile, p.childRows, p.cols); err != nil {
		return err
	}
	if err := p.applyChildViewport(); err != nil {
		return err
	}
	return p.drawBar()
}

func (p *instance) cleanup() {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("\x1b[0m")
	buf.WriteString("\x1b[r")
	if p.barHeight > 0 {
		for i := 0; i < p.barHeight; i++ {
			buf.WriteString(ansi.FormatCUP(p.childRows+1+i, 1))
			buf.WriteString("\x1b[2K")
		}
	}
	buf.WriteString(ansi.FormatCUP(ansi.Clamp(p.tracker.Row, 1, p.rows), p.tracker.Col))
	_, _ = p.stdout.Write(buf.Bytes())
}

func waitChild(pid int, ch chan<- exitStatus) {
	var status syscall.WaitStatus
	_, err := syscall.Wait4(pid, &status, 0, nil)
	ch <- exitStatus{status: status, err: err}
}

func copyInput(dst *os.File, src *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, _ = dst.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func readPTY(src *os.File, out chan<- []byte) {
	defer close(out)
	buf := make([]byte, 8192)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			out <- chunk
		}
		if err != nil {
			return
		}
	}
}

func colorLine(text, bg, fg string) string {
	return fmt.Sprintf("\x1b[%s;%sm%s\x1b[0m", bg, fg, text)
}

func fitWidth(text string, width int) string {
	runes := []rune(text)
	if len(runes) > width {
		if width <= 1 {
			return string(runes[:width])
		}
		runes = append(runes[:width-1], '…')
	}
	if len(runes) < width {
		runes = append(runes, []rune(strings.Repeat(" ", width-len(runes)))...)
	}
	return string(runes)
}

func wrapText(text string, width int) []string {
	if width <= 1 {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	lines := []string{}
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if len([]rune(candidate)) <= width {
			current = candidate
			continue
		}
		lines = append(lines, fitWidth(current, width))
		current = word
	}
	lines = append(lines, fitWidth(current, width))
	return lines
}
