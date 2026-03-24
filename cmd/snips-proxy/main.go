package main

/*
#cgo linux LDFLAGS: -lutil
#include <errno.h>
#include <pty.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/wait.h>
#include <termios.h>
#include <unistd.h>

struct fork_result {
	int master_fd;
	pid_t child_pid;
	int err;
};

static struct fork_result fork_exec(char **argv, unsigned short rows, unsigned short cols) {
	struct fork_result result;
	struct winsize ws;
	memset(&result, 0, sizeof(result));
	memset(&ws, 0, sizeof(ws));
	ws.ws_row = rows;
	ws.ws_col = cols;

	int master = -1;
	pid_t pid = forkpty(&master, NULL, NULL, &ws);
	if (pid < 0) {
		result.err = errno;
		return result;
	}
	if (pid == 0) {
		execvp(argv[0], argv);
		_exit(127);
	}

	result.master_fd = master;
	result.child_pid = pid;
	return result;
}

static int get_winsize(int fd, unsigned short *rows, unsigned short *cols) {
	struct winsize ws;
	if (ioctl(fd, TIOCGWINSZ, &ws) != 0) {
		return errno;
	}
	*rows = ws.ws_row;
	*cols = ws.ws_col;
	return 0;
}

static int set_winsize(int fd, unsigned short rows, unsigned short cols) {
	struct winsize ws;
	memset(&ws, 0, sizeof(ws));
	ws.ws_row = rows;
	ws.ws_col = cols;
	if (ioctl(fd, TIOCSWINSZ, &ws) != 0) {
		return errno;
	}
	return 0;
}

static int make_raw(int fd, struct termios *original) {
	struct termios raw;
	if (tcgetattr(fd, original) != 0) {
		return errno;
	}
	raw = *original;
	cfmakeraw(&raw);
	if (tcsetattr(fd, TCSANOW, &raw) != 0) {
		return errno;
	}
	return 0;
}

static int restore_termios(int fd, struct termios *original) {
	if (tcsetattr(fd, TCSANOW, original) != 0) {
		return errno;
	}
	return 0;
}

static int signal_process_group(pid_t pid, int sig) {
	if (kill(-pid, sig) != 0) {
		return errno;
	}
	return 0;
}
*/
import "C"

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	defaultInterval = 30 * time.Second
	defaultHeight   = 2
)

type snippet struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

type snippetStore struct {
	keys       []string
	snippets   map[string]snippet
	recent     []string
	maxRecent  int
	randomizer *rand.Rand
}

type colorTheme struct {
	headerBG string
	headerFG string
	bodyBG   string
	bodyFG   string
}

var colorThemes = []colorTheme{
	{headerBG: "48;5;25", headerFG: "38;5;231", bodyBG: "48;5;159", bodyFG: "38;5;16"},
	{headerBG: "48;5;88", headerFG: "38;5;230", bodyBG: "48;5;223", bodyFG: "38;5;16"},
	{headerBG: "48;5;22", headerFG: "38;5;230", bodyBG: "48;5;157", bodyFG: "38;5;16"},
	{headerBG: "48;5;60", headerFG: "38;5;231", bodyBG: "48;5;189", bodyFG: "38;5;16"},
	{headerBG: "48;5;130", headerFG: "38;5;230", bodyBG: "48;5;223", bodyFG: "38;5;16"},
	{headerBG: "48;5;24", headerFG: "38;5;230", bodyBG: "48;5;117", bodyFG: "38;5;16"},
}

func loadSnippets(path string) (*snippetStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	snippets := make(map[string]snippet)
	if err := json.Unmarshal(data, &snippets); err != nil {
		return nil, err
	}
	if len(snippets) == 0 {
		return nil, errors.New("snippets file is empty")
	}

	keys := make([]string, 0, len(snippets))
	for key := range snippets {
		keys = append(keys, key)
	}

	return &snippetStore{
		keys:       keys,
		snippets:   snippets,
		maxRecent:  min(10, len(keys)),
		randomizer: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (s *snippetStore) next() snippet {
	candidates := make([]string, 0, len(s.keys))
	for _, key := range s.keys {
		if !containsString(s.recent, key) {
			candidates = append(candidates, key)
		}
	}
	if len(candidates) == 0 {
		s.recent = s.recent[:0]
		candidates = append(candidates, s.keys...)
	}

	key := candidates[s.randomizer.Intn(len(candidates))]
	s.recent = append(s.recent, key)
	if len(s.recent) > s.maxRecent {
		s.recent = s.recent[1:]
	}

	item := s.snippets[key]
	if item.Term == "" {
		item.Term = key
	}
	if item.Definition == "" {
		item.Definition = "No definition available."
	}
	return item
}

type tracker struct {
	rows         int
	cols         int
	row          int
	col          int
	savedRow     int
	savedCol     int
	scrollTop    int
	scrollBottom int
}

func newTracker(rows, cols, scrollTop, scrollBottom int) *tracker {
	return &tracker{
		rows:         rows,
		cols:         cols,
		row:          scrollTop,
		col:          1,
		savedRow:     scrollTop,
		savedCol:     1,
		scrollTop:    scrollTop,
		scrollBottom: scrollBottom,
	}
}

func (t *tracker) resize(rows, cols, scrollTop, scrollBottom int) {
	t.rows = rows
	t.cols = cols
	t.scrollTop = scrollTop
	t.scrollBottom = scrollBottom
	t.row = clamp(t.row, scrollTop, scrollBottom)
	t.col = clamp(t.col, 1, max(1, cols))
	t.savedRow = clamp(t.savedRow, scrollTop, scrollBottom)
	t.savedCol = clamp(t.savedCol, 1, max(1, cols))
}

func (t *tracker) save() {
	t.savedRow = t.row
	t.savedCol = t.col
}

func (t *tracker) restore() {
	t.row = clamp(t.savedRow, 1, t.rows)
	t.col = clamp(t.savedCol, 1, max(1, t.cols))
}

func (t *tracker) setCursor(row, col int) {
	t.row = clamp(row, 1, t.rows)
	t.col = clamp(col, 1, max(1, t.cols))
}

func (t *tracker) setScrollRegion(top, bottom int) {
	top = clamp(top, 1, t.rows)
	bottom = clamp(bottom, top, t.rows)
	t.scrollTop = top
	t.scrollBottom = bottom
	t.row = top
	t.col = 1
}

func (t *tracker) moveDown(lines int) {
	if lines <= 0 {
		return
	}
	t.row += lines
	if t.row > t.scrollBottom {
		t.row = t.scrollBottom
	}
}

func (t *tracker) moveUp(lines int) {
	if lines <= 0 {
		return
	}
	t.row -= lines
	if t.row < t.scrollTop {
		t.row = t.scrollTop
	}
}

func (t *tracker) moveRight(cols int) {
	if cols <= 0 {
		return
	}
	t.col += cols
	if t.col > t.cols {
		t.col = t.cols
	}
}

func (t *tracker) moveLeft(cols int) {
	if cols <= 0 {
		return
	}
	t.col -= cols
	if t.col < 1 {
		t.col = 1
	}
}

func (t *tracker) printable(width int) {
	if width <= 0 {
		width = 1
	}
	t.col += width
	if t.col > t.cols {
		t.col = t.cols
	}
}

func (t *tracker) lineFeed() {
	if t.row >= t.scrollBottom {
		t.row = t.scrollBottom
		return
	}
	t.row++
}

func (t *tracker) reverseIndex() {
	if t.row <= t.scrollTop {
		t.row = t.scrollTop
		return
	}
	t.row--
}

type proxy struct {
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
	store           *snippetStore
	currentSnippet  snippet
	currentTheme    colorTheme
	altScreenActive bool
	altSavedRow     int
	altSavedCol     int
	tracker         *tracker
	rewriter           *ansiRewriter
}

func newProxy(master *os.File, childPID int, stdinFD int, stdout io.Writer, requestedHeight int, store *snippetStore) *proxy {
	return &proxy{
		masterFile:      master,
		childPID:        childPID,
		stdinFD:         stdinFD,
		stdout:          stdout,
		requestedHeight: max(0, requestedHeight),
		store:           store,
	}
}

func (p *proxy) updateLayout(rows, cols int) {
	p.rows = max(1, rows)
	p.cols = max(20, cols)
	maxBar := max(0, p.rows-1)
	p.barHeight = min(p.requestedHeight, maxBar)
	p.childTop = 1
	p.childRows = max(1, p.rows-p.barHeight)
	if p.tracker == nil {
		p.tracker = newTracker(p.rows, p.cols, 1, p.childRows)
	} else {
		p.tracker.resize(p.rows, p.cols, 1, p.childRows)
	}
	if p.rewriter == nil {
		p.rewriter = newANSIRewriter(p)
	} else {
		p.rewriter.updateLayout()
	}
}

func (p *proxy) nextSnippet() {
	p.currentSnippet = p.store.next()
	p.currentTheme = colorThemes[p.store.randomizer.Intn(len(colorThemes))]
}

func (p *proxy) saveAltCursor() {
	p.altSavedRow = p.tracker.row
	p.altSavedCol = p.tracker.col
}

func (p *proxy) restoreAltCursor() {
	if p.altSavedRow == 0 {
		p.tracker.setCursor(p.childTop, 1)
		return
	}
	p.tracker.setCursor(p.altSavedRow, p.altSavedCol)
}

func (p *proxy) initializeTerminal() error {
	if err := p.applyChildViewport(); err != nil {
		return err
	}
	return p.drawBar()
}

func (p *proxy) applyChildViewport() error {
	seq := bytes.NewBuffer(nil)
	seq.WriteString("\x1b[0m")
	seq.WriteString("\x1b[r")
	if p.barHeight > 0 {
		seq.WriteString(fmt.Sprintf("\x1b[1;%dr", p.childRows))
	}
	seq.WriteString(formatCUP(1, 1))
	_, err := p.stdout.Write(seq.Bytes())
	return err
}

func (p *proxy) drawBar() error {
	if p.currentSnippet.Term == "" {
		p.nextSnippet()
	}

	lines := p.renderBar()

	buf := bytes.NewBuffer(nil)
	buf.WriteString("\x1b[0m")
	buf.WriteString("\x1b[r")
	for i := 0; i < p.barHeight; i++ {
		row := p.childRows + 1 + i
		buf.WriteString(formatCUP(row, 1))
		buf.WriteString("\x1b[2K")
		if i < len(lines) {
			buf.WriteString(lines[i])
		}
		buf.WriteString("\x1b[0m")
	}
	if p.barHeight > 0 {
		buf.WriteString(fmt.Sprintf("\x1b[1;%dr", p.childRows))
	}
	buf.WriteString(formatCUP(p.tracker.row, p.tracker.col))
	_, err := p.stdout.Write(buf.Bytes())
	return err
}

func (p *proxy) renderBar() []string {
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

func (p *proxy) handleChildOutput(data []byte) error {
	out := p.rewriter.feed(data)
	if len(out) == 0 {
		return nil
	}
	_, err := p.stdout.Write(out)
	return err
}

func (p *proxy) resize() error {
	rows, cols, err := terminalSize(p.stdinFD)
	if err != nil {
		return err
	}
	p.updateLayout(rows, cols)
	if err := setPTYSize(int(p.masterFile.Fd()), p.childRows, p.cols); err != nil {
		return err
	}
	if err := p.applyChildViewport(); err != nil {
		return err
	}
	return p.drawBar()
}

func (p *proxy) cleanup() {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("\x1b[0m")
	buf.WriteString("\x1b[r")
	if p.barHeight > 0 {
		for i := 0; i < p.barHeight; i++ {
			buf.WriteString(formatCUP(p.childRows+1+i, 1))
			buf.WriteString("\x1b[2K")
		}
	}
	buf.WriteString(formatCUP(clamp(p.tracker.row, 1, p.rows), p.tracker.col))
	_, _ = p.stdout.Write(buf.Bytes())
}

type ansiRewriter struct {
	proxy  *proxy
	state  parserState
	seq    []byte
	oscEsc bool
	stEsc  bool
}

type parserState int

const (
	stateGround parserState = iota
	stateEscape
	stateCSI
	stateOSC
	stateString
)

func newANSIRewriter(proxy *proxy) *ansiRewriter {
	return &ansiRewriter{
		proxy: proxy,
		seq:   make([]byte, 0, 64),
	}
}

func (r *ansiRewriter) updateLayout() {}

func (r *ansiRewriter) feed(data []byte) []byte {
	var out bytes.Buffer
	for _, b := range data {
		switch r.state {
		case stateGround:
			r.handleGroundByte(&out, b)
		case stateEscape:
			r.handleEscapeByte(&out, b)
		case stateCSI:
			r.handleCSIByte(&out, b)
		case stateOSC:
			r.handleOSCByte(&out, b)
		case stateString:
			r.handleStringByte(&out, b)
		}
	}
	return out.Bytes()
}

func (r *ansiRewriter) handleGroundByte(out *bytes.Buffer, b byte) {
	switch b {
	case 0x1b:
		r.state = stateEscape
		r.seq = append(r.seq[:0], b)
	case '\r':
		r.proxy.tracker.col = 1
		out.WriteByte(b)
	case '\n':
		r.proxy.tracker.lineFeed()
		out.WriteByte(b)
	case '\b':
		r.proxy.tracker.moveLeft(1)
		out.WriteByte(b)
	case 0x07, '\t':
		out.WriteByte(b)
	default:
		if b < 0x20 || b == 0x7f {
			out.WriteByte(b)
			return
		}
		r.proxy.tracker.printable(1)
		out.WriteByte(b)
	}
}

func (r *ansiRewriter) handleEscapeByte(out *bytes.Buffer, b byte) {
	r.seq = append(r.seq, b)
	switch b {
	case '[':
		r.state = stateCSI
	case ']':
		r.state = stateOSC
		r.oscEsc = false
	case 'P', '^', '_':
		r.state = stateString
		r.stEsc = false
	case '7':
		r.proxy.tracker.save()
		out.Write(r.seq)
		r.state = stateGround
	case '8':
		r.proxy.tracker.restore()
		out.Write(r.seq)
		r.state = stateGround
	case 'D':
		r.proxy.tracker.lineFeed()
		out.Write(r.seq)
		r.state = stateGround
	case 'M':
		r.proxy.tracker.reverseIndex()
		out.Write(r.seq)
		r.state = stateGround
	case 'E':
		r.proxy.tracker.lineFeed()
		r.proxy.tracker.col = 1
		out.Write(r.seq)
		r.state = stateGround
	default:
		out.Write(r.seq)
		r.state = stateGround
	}
}

func (r *ansiRewriter) handleCSIByte(out *bytes.Buffer, b byte) {
	r.seq = append(r.seq, b)
	if b >= 0x40 && b <= 0x7e {
		out.Write(r.rewriteCSI(r.seq))
		r.state = stateGround
	}
}

func (r *ansiRewriter) handleOSCByte(out *bytes.Buffer, b byte) {
	r.seq = append(r.seq, b)
	if b == 0x07 {
		out.Write(r.seq)
		r.state = stateGround
		return
	}
	if r.oscEsc && b == '\\' {
		out.Write(r.seq)
		r.state = stateGround
		r.oscEsc = false
		return
	}
	r.oscEsc = b == 0x1b
}

func (r *ansiRewriter) handleStringByte(out *bytes.Buffer, b byte) {
	r.seq = append(r.seq, b)
	if r.stEsc && b == '\\' {
		out.Write(r.seq)
		r.state = stateGround
		r.stEsc = false
		return
	}
	r.stEsc = b == 0x1b
}

func (r *ansiRewriter) rewriteCSI(seq []byte) []byte {
	if len(seq) < 3 {
		return append([]byte(nil), seq...)
	}

	final := seq[len(seq)-1]
	body := string(seq[2 : len(seq)-1])
	private, params := parseCSI(body)

	switch final {
	case 'H', 'f':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		row := paramOr(params, 0, 1)
		col := paramOr(params, 1, 1)
		row = clamp(row, 1, r.proxy.childRows)
		col = clamp(col, 1, r.proxy.cols)
		actualRow := r.proxy.childTop - 1 + row
		r.proxy.tracker.setCursor(actualRow, col)
		return []byte(formatCUP(actualRow, col))
	case 'd':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		row := clamp(paramOr(params, 0, 1), 1, r.proxy.childRows)
		actualRow := r.proxy.childTop - 1 + row
		r.proxy.tracker.setCursor(actualRow, r.proxy.tracker.col)
		return []byte(fmt.Sprintf("\x1b[%dd", actualRow))
	case 'G':
		if private == "" {
			r.proxy.tracker.col = clamp(paramOr(params, 0, 1), 1, r.proxy.cols)
		}
		return append([]byte(nil), seq...)
	case 'A':
		if private == "" {
			r.proxy.tracker.moveUp(paramOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'B':
		if private == "" {
			r.proxy.tracker.moveDown(paramOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'C':
		if private == "" {
			r.proxy.tracker.moveRight(paramOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'D':
		if private == "" {
			r.proxy.tracker.moveLeft(paramOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'E':
		if private == "" {
			r.proxy.tracker.moveDown(paramOr(params, 0, 1))
			r.proxy.tracker.col = 1
		}
		return append([]byte(nil), seq...)
	case 'F':
		if private == "" {
			r.proxy.tracker.moveUp(paramOr(params, 0, 1))
			r.proxy.tracker.col = 1
		}
		return append([]byte(nil), seq...)
	case 'J':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		return r.rewriteEraseDisplay(paramOr(params, 0, 0))
	case 'K':
		return append([]byte(nil), seq...)
	case 's':
		if private == "" {
			r.proxy.tracker.save()
		}
		return append([]byte(nil), seq...)
	case 'u':
		if private == "" {
			r.proxy.tracker.restore()
		}
		return append([]byte(nil), seq...)
	case 'r':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		top := paramOr(params, 0, 1)
		bottom := paramOr(params, 1, r.proxy.childRows)
		top = clamp(top, 1, r.proxy.childRows)
		bottom = clamp(bottom, top, r.proxy.childRows)
		actualTop := r.proxy.childTop - 1 + top
		actualBottom := r.proxy.childTop - 1 + bottom
		r.proxy.tracker.setScrollRegion(actualTop, actualBottom)
		return []byte(fmt.Sprintf("\x1b[%d;%dr", actualTop, actualBottom))
	case 'h', 'l':
		if private == "?" {
			return r.rewritePrivateMode(params, final)
		}
		return append([]byte(nil), seq...)
	default:
		return append([]byte(nil), seq...)
	}
}

func (r *ansiRewriter) rewritePrivateMode(params []int, final byte) []byte {
	filtered := make([]int, 0, len(params))
	var out bytes.Buffer

	for _, param := range params {
		switch param {
		case 47, 1047:
			out.WriteString(r.emulateAltScreen(final, false))
		case 1048:
			out.WriteString(r.emulateAltCursor(final))
		case 1049:
			out.WriteString(r.emulateAltScreen(final, true))
		default:
			filtered = append(filtered, param)
		}
	}

	if len(filtered) > 0 {
		out.WriteString(formatPrivateCSI("?", filtered, final))
	}
	return out.Bytes()
}

func (r *ansiRewriter) emulateAltCursor(final byte) string {
	if final == 'h' {
		r.proxy.saveAltCursor()
		return ""
	}
	r.proxy.restoreAltCursor()
	return formatCUP(r.proxy.tracker.row, r.proxy.tracker.col)
}

func (r *ansiRewriter) emulateAltScreen(final byte, saveCursor bool) string {
	if final == 'h' {
		if saveCursor {
			r.proxy.saveAltCursor()
		}
		r.proxy.altScreenActive = true
		r.proxy.tracker.setScrollRegion(1, r.proxy.childRows)
		r.proxy.tracker.setCursor(1, 1)
		return r.clearChildViewport() +
			fmt.Sprintf("\x1b[1;%dr", r.proxy.childRows) +
			formatCUP(1, 1)
	}

	clear := r.clearChildViewport() + fmt.Sprintf("\x1b[1;%dr", r.proxy.childRows)
	if saveCursor {
		r.proxy.restoreAltCursor()
		r.proxy.altScreenActive = false
		return clear + formatCUP(r.proxy.tracker.row, r.proxy.tracker.col)
	}
	r.proxy.tracker.setCursor(1, 1)
	r.proxy.altScreenActive = false
	return clear + formatCUP(1, 1)
}

func (r *ansiRewriter) rewriteEraseDisplay(mode int) []byte {
	switch mode {
	case 0:
		return []byte(r.clearFromCursorDown())
	case 1:
		return []byte(r.clearToCursor())
	case 2, 3:
		return []byte(r.clearChildViewport())
	default:
		return []byte(fmt.Sprintf("\x1b[%dJ", mode))
	}
}

func (r *ansiRewriter) clearToCursor() string {
	buf := bytes.NewBuffer(nil)
	cursorRow := r.proxy.tracker.row
	cursorCol := r.proxy.tracker.col
	for row := r.proxy.childTop; row < cursorRow; row++ {
		buf.WriteString(formatCUP(row, 1))
		buf.WriteString("\x1b[2K")
	}
	buf.WriteString(formatCUP(cursorRow, cursorCol))
	buf.WriteString("\x1b[1K")
	buf.WriteString(formatCUP(cursorRow, cursorCol))
	return buf.String()
}

func (r *ansiRewriter) clearChildViewport() string {
	buf := bytes.NewBuffer(nil)
	cursorRow := r.proxy.tracker.row
	cursorCol := r.proxy.tracker.col
	for row := 1; row <= r.proxy.childRows; row++ {
		buf.WriteString(formatCUP(row, 1))
		buf.WriteString("\x1b[2K")
	}
	buf.WriteString(formatCUP(cursorRow, cursorCol))
	return buf.String()
}

func (r *ansiRewriter) clearFromCursorDown() string {
	cursorRow := r.proxy.tracker.row
	cursorCol := r.proxy.tracker.col
	if cursorRow >= r.proxy.childRows {
		return "\x1b[0K"
	}
	buf := bytes.NewBuffer(nil)
	buf.WriteString("\x1b[0K")
	for row := cursorRow + 1; row <= r.proxy.childRows; row++ {
		buf.WriteString(formatCUP(row, 1))
		buf.WriteString("\x1b[2K")
	}
	buf.WriteString(formatCUP(cursorRow, cursorCol))
	return buf.String()
}

func parseCSI(body string) (string, []int) {
	if body == "" {
		return "", nil
	}
	private := ""
	if strings.ContainsAny(string(body[0]), "?><=!") {
		private = string(body[0])
		body = body[1:]
	}
	if body == "" {
		return private, nil
	}

	parts := strings.Split(body, ";")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			values = append(values, 0)
			continue
		}
		var value int
		fmt.Sscanf(part, "%d", &value)
		values = append(values, value)
	}
	return private, values
}

func paramOr(params []int, index, fallback int) int {
	if index >= len(params) || params[index] == 0 {
		return fallback
	}
	return params[index]
}

type terminalMode struct {
	fd       int
	original C.struct_termios
	restored bool
	mu       sync.Mutex
}

func enableRawMode(fd int) (*terminalMode, error) {
	mode := &terminalMode{fd: fd}
	if err := errnoFromC(C.make_raw(C.int(fd), &mode.original)); err != nil {
		return nil, err
	}
	return mode, nil
}

func (m *terminalMode) restore() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.restored {
		return
	}
	_ = errnoFromC(C.restore_termios(C.int(m.fd), &m.original))
	m.restored = true
}

func errnoFromC(code C.int) error {
	if code == 0 {
		return nil
	}
	return syscall.Errno(code)
}

func terminalSize(fd int) (int, int, error) {
	var rows C.ushort
	var cols C.ushort
	if err := errnoFromC(C.get_winsize(C.int(fd), &rows, &cols)); err != nil {
		return 0, 0, err
	}
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	return int(rows), int(cols), nil
}

func setPTYSize(fd int, rows, cols int) error {
	return errnoFromC(C.set_winsize(C.int(fd), C.ushort(rows), C.ushort(cols)))
}

func forkChild(argv []string, rows, cols int) (*os.File, int, error) {
	cargv := make([]*C.char, 0, len(argv)+1)
	for _, arg := range argv {
		cargv = append(cargv, C.CString(arg))
	}
	cargv = append(cargv, nil)
	defer func() {
		for _, arg := range cargv[:len(cargv)-1] {
			C.free(unsafe.Pointer(arg))
		}
	}()

	result := C.fork_exec((**C.char)(unsafe.Pointer(&cargv[0])), C.ushort(rows), C.ushort(cols))
	if result.err != 0 {
		return nil, 0, syscall.Errno(result.err)
	}
	return os.NewFile(uintptr(result.master_fd), "pty-master"), int(result.child_pid), nil
}

func signalChildGroup(pid int, sig syscall.Signal) {
	_ = errnoFromC(C.signal_process_group(C.int(pid), C.int(sig)))
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

func formatCUP(row, col int) string {
	return fmt.Sprintf("\x1b[%d;%dH", row, col)
}

func formatPrivateCSI(private string, params []int, final byte) string {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		parts = append(parts, fmt.Sprintf("%d", param))
	}
	return fmt.Sprintf("\x1b[%s%s%c", private, strings.Join(parts, ";"), final)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func defaultSnippetsPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "snippets.json"
	}
	return filepath.Join(filepath.Dir(exe), "snippets.json")
}

func defaultShell() string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}

type exitStatus struct {
	status syscall.WaitStatus
	err    error
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

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: snips wrap [--height N] [--interval SECONDS] [--snippets-file PATH] [-- command ...]\n")
}

func main() {
	var (
		barHeight    int
		intervalSecs int
		snippetsPath string
	)

	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.IntVar(&barHeight, "height", defaultHeight, "bar height in terminal rows")
	flags.IntVar(&intervalSecs, "interval", int(defaultInterval.Seconds()), "snippet rotation interval")
	flags.StringVar(&snippetsPath, "snippets-file", defaultSnippetsPath(), "path to snippets JSON file")
	flags.StringVar(&snippetsPath, "file", defaultSnippetsPath(), "path to snippets JSON file")

	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage()
			os.Exit(0)
		}
		usage()
		os.Exit(2)
	}

	if !isTTY(os.Stdin.Fd()) || !isTTY(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr, "snips proxy mode requires a real TTY on stdin/stdout")
		os.Exit(2)
	}

	store, err := loadSnippets(snippetsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load snippets: %v\n", err)
		os.Exit(1)
	}

	command := flags.Args()
	if len(command) == 0 {
		command = []string{defaultShell()}
	}

	rows, cols, err := terminalSize(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read terminal size: %v\n", err)
		os.Exit(1)
	}

	effectiveBar := min(max(0, barHeight), max(0, rows-1))
	childRows := max(1, rows-effectiveBar)
	master, pid, err := forkChild(command, childRows, cols)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start child PTY: %v\n", err)
		os.Exit(1)
	}
	defer master.Close()

	mode, err := enableRawMode(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to switch terminal to raw mode: %v\n", err)
		os.Exit(1)
	}
	defer mode.restore()

	proxy := newProxy(master, pid, int(os.Stdin.Fd()), os.Stdout, barHeight, store)
	proxy.updateLayout(rows, cols)
	proxy.nextSnippet()
	if err := proxy.initializeTerminal(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize proxy terminal: %v\n", err)
		os.Exit(1)
	}
	defer proxy.cleanup()

	outputCh := make(chan []byte, 32)
	exitCh := make(chan exitStatus, 1)
	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)
	defer signal.Stop(resizeCh)

	go readPTY(master, outputCh)
	go waitChild(pid, exitCh)
	go copyInput(master, os.Stdin)

	rotateTicker := time.NewTicker(time.Duration(max(1, intervalSecs)) * time.Second)
	defer rotateTicker.Stop()

	var (
		readerClosed bool
		childExited  bool
		childStatus  exitStatus
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
				break loop
			}
		case <-rotateTicker.C:
			proxy.nextSnippet()
			if err := proxy.drawBar(); err != nil {
				break loop
			}
		case <-resizeCh:
			if err := proxy.resize(); err != nil {
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

	signalChildGroup(pid, syscall.SIGHUP)

	// Explicitly clean up before os.Exit (which skips defers).
	proxy.cleanup()
	mode.restore()

	if childStatus.err != nil {
		fmt.Fprintf(os.Stderr, "child wait failed: %v\n", childStatus.err)
		os.Exit(1)
	}
	if childStatus.status.Exited() {
		os.Exit(childStatus.status.ExitStatus())
	}
	if childStatus.status.Signaled() {
		os.Exit(128 + int(childStatus.status.Signal()))
	}
}

func isTTY(fd uintptr) bool {
	return C.isatty(C.int(fd)) == 1
}
