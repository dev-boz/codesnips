package ansi

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

type Layout struct {
	ChildTop  int
	ChildRows int
	Cols      int
}

type Callbacks struct {
	SaveAltCursor    func()
	RestoreAltCursor func()
}

type Rewriter struct {
	tracker   *Tracker
	layout    Layout
	callbacks Callbacks

	state      parserState
	seq        []byte
	oscEsc     bool
	stEsc      bool
	utfPending []byte
}

type parserState int

const (
	stateGround parserState = iota
	stateEscape
	stateCSI
	stateOSC
	stateString
)

func NewRewriter(tracker *Tracker, layout Layout, callbacks Callbacks) *Rewriter {
	return &Rewriter{
		tracker:    tracker,
		layout:     layout,
		callbacks:  callbacks,
		seq:        make([]byte, 0, 64),
		utfPending: make([]byte, 0, utf8.UTFMax),
	}
}

func (r *Rewriter) UpdateLayout(layout Layout) {
	r.layout = layout
}

func (r *Rewriter) Feed(data []byte) []byte {
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

func (r *Rewriter) handleGroundByte(out *bytes.Buffer, b byte) {
	switch b {
	case 0x1b:
		r.flushPendingPrintable()
		r.state = stateEscape
		r.seq = append(r.seq[:0], b)
	case '\r':
		r.flushPendingPrintable()
		r.tracker.SetCol(1)
		out.WriteByte(b)
	case '\n':
		r.flushPendingPrintable()
		r.tracker.LineFeed()
		out.WriteByte(b)
	case '\b':
		r.flushPendingPrintable()
		r.tracker.MoveLeft(1)
		out.WriteByte(b)
	case 0x07, '\t':
		r.flushPendingPrintable()
		out.WriteByte(b)
	default:
		if b < 0x20 || b == 0x7f {
			r.flushPendingPrintable()
			out.WriteByte(b)
			return
		}
		out.WriteByte(b)
		r.consumePrintableByte(b)
	}
}

func (r *Rewriter) consumePrintableByte(b byte) {
	r.utfPending = append(r.utfPending, b)
	for len(r.utfPending) > 0 {
		if r.utfPending[0] < utf8.RuneSelf {
			r.tracker.Printable(runewidth.RuneWidth(rune(r.utfPending[0])))
			r.utfPending = r.utfPending[1:]
			continue
		}
		if !utf8.FullRune(r.utfPending) {
			if len(r.utfPending) > utf8.UTFMax {
				r.tracker.Printable(1)
				r.utfPending = r.utfPending[1:]
			}
			return
		}
		decoded, size := utf8.DecodeRune(r.utfPending)
		if size <= 0 {
			return
		}
		width := runewidth.RuneWidth(decoded)
		if width <= 0 {
			width = 1
		}
		r.tracker.Printable(width)
		r.utfPending = r.utfPending[size:]
	}
}

func (r *Rewriter) flushPendingPrintable() {
	for len(r.utfPending) > 0 {
		if r.utfPending[0] < utf8.RuneSelf {
			r.tracker.Printable(runewidth.RuneWidth(rune(r.utfPending[0])))
			r.utfPending = r.utfPending[1:]
			continue
		}
		if !utf8.FullRune(r.utfPending) {
			r.tracker.Printable(1)
			r.utfPending = r.utfPending[1:]
			continue
		}
		decoded, size := utf8.DecodeRune(r.utfPending)
		width := runewidth.RuneWidth(decoded)
		if width <= 0 {
			width = 1
		}
		r.tracker.Printable(width)
		r.utfPending = r.utfPending[size:]
	}
}

func (r *Rewriter) handleEscapeByte(out *bytes.Buffer, b byte) {
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
		r.tracker.Save()
		out.Write(r.seq)
		r.state = stateGround
	case '8':
		r.tracker.Restore()
		out.Write(r.seq)
		r.state = stateGround
	case 'D':
		r.tracker.LineFeed()
		out.Write(r.seq)
		r.state = stateGround
	case 'M':
		r.tracker.ReverseIndex()
		out.Write(r.seq)
		r.state = stateGround
	case 'E':
		r.tracker.LineFeed()
		r.tracker.SetCol(1)
		out.Write(r.seq)
		r.state = stateGround
	default:
		out.Write(r.seq)
		r.state = stateGround
	}
}

func (r *Rewriter) handleCSIByte(out *bytes.Buffer, b byte) {
	r.seq = append(r.seq, b)
	if b >= 0x40 && b <= 0x7e {
		out.Write(r.rewriteCSI(r.seq))
		r.state = stateGround
	}
}

func (r *Rewriter) handleOSCByte(out *bytes.Buffer, b byte) {
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

func (r *Rewriter) handleStringByte(out *bytes.Buffer, b byte) {
	r.seq = append(r.seq, b)
	if r.stEsc && b == '\\' {
		out.Write(r.seq)
		r.state = stateGround
		r.stEsc = false
		return
	}
	r.stEsc = b == 0x1b
}

func (r *Rewriter) rewriteCSI(seq []byte) []byte {
	if len(seq) < 3 {
		return append([]byte(nil), seq...)
	}

	final := seq[len(seq)-1]
	body := string(seq[2 : len(seq)-1])
	private, params := ParseCSI(body)

	switch final {
	case 'H', 'f':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		row := ParamOr(params, 0, 1)
		col := ParamOr(params, 1, 1)
		row = Clamp(row, 1, r.layout.ChildRows)
		col = Clamp(col, 1, r.layout.Cols)
		actualRow := r.layout.ChildTop - 1 + row
		r.tracker.SetCursor(actualRow, col)
		return []byte(FormatCUP(actualRow, col))
	case 'd':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		row := Clamp(ParamOr(params, 0, 1), 1, r.layout.ChildRows)
		actualRow := r.layout.ChildTop - 1 + row
		r.tracker.SetCursor(actualRow, r.tracker.Col)
		return []byte(fmt.Sprintf("\x1b[%dd", actualRow))
	case 'G':
		if private == "" {
			r.tracker.SetCol(Clamp(ParamOr(params, 0, 1), 1, r.layout.Cols))
		}
		return append([]byte(nil), seq...)
	case 'A':
		if private == "" {
			r.tracker.MoveUp(ParamOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'B':
		if private == "" {
			r.tracker.MoveDown(ParamOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'C':
		if private == "" {
			r.tracker.MoveRight(ParamOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'D':
		if private == "" {
			r.tracker.MoveLeft(ParamOr(params, 0, 1))
		}
		return append([]byte(nil), seq...)
	case 'E':
		if private == "" {
			r.tracker.MoveDown(ParamOr(params, 0, 1))
			r.tracker.SetCol(1)
		}
		return append([]byte(nil), seq...)
	case 'F':
		if private == "" {
			r.tracker.MoveUp(ParamOr(params, 0, 1))
			r.tracker.SetCol(1)
		}
		return append([]byte(nil), seq...)
	case 'J':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		return r.rewriteEraseDisplay(ParamOr(params, 0, 0))
	case 'K':
		return append([]byte(nil), seq...)
	case 's':
		if private == "" {
			r.tracker.Save()
		}
		return append([]byte(nil), seq...)
	case 'u':
		if private == "" {
			r.tracker.Restore()
		}
		return append([]byte(nil), seq...)
	case 'r':
		if private != "" {
			return append([]byte(nil), seq...)
		}
		top := ParamOr(params, 0, 1)
		bottom := ParamOr(params, 1, r.layout.ChildRows)
		top = Clamp(top, 1, r.layout.ChildRows)
		bottom = Clamp(bottom, top, r.layout.ChildRows)
		actualTop := r.layout.ChildTop - 1 + top
		actualBottom := r.layout.ChildTop - 1 + bottom
		r.tracker.SetScrollRegion(actualTop, actualBottom)
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

func (r *Rewriter) rewritePrivateMode(params []int, final byte) []byte {
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
		out.WriteString(FormatPrivateCSI("?", filtered, final))
	}
	return out.Bytes()
}

func (r *Rewriter) emulateAltCursor(final byte) string {
	if final == 'h' {
		if r.callbacks.SaveAltCursor != nil {
			r.callbacks.SaveAltCursor()
		}
		return ""
	}
	if r.callbacks.RestoreAltCursor != nil {
		r.callbacks.RestoreAltCursor()
	}
	return FormatCUP(r.tracker.Row, r.tracker.Col)
}

func (r *Rewriter) emulateAltScreen(final byte, saveCursor bool) string {
	if final == 'h' {
		if saveCursor && r.callbacks.SaveAltCursor != nil {
			r.callbacks.SaveAltCursor()
		}
		r.tracker.SetScrollRegion(1, r.layout.ChildRows)
		r.tracker.SetCursor(1, 1)
		return r.clearChildViewport() +
			fmt.Sprintf("\x1b[1;%dr", r.layout.ChildRows) +
			FormatCUP(1, 1)
	}

	clear := r.clearChildViewport() + fmt.Sprintf("\x1b[1;%dr", r.layout.ChildRows)
	if saveCursor {
		if r.callbacks.RestoreAltCursor != nil {
			r.callbacks.RestoreAltCursor()
		}
		return clear + FormatCUP(r.tracker.Row, r.tracker.Col)
	}
	r.tracker.SetCursor(1, 1)
	return clear + FormatCUP(1, 1)
}

func (r *Rewriter) rewriteEraseDisplay(mode int) []byte {
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

func (r *Rewriter) clearToCursor() string {
	buf := bytes.NewBuffer(nil)
	cursorRow := r.tracker.Row
	cursorCol := r.tracker.Col
	for row := r.layout.ChildTop; row < cursorRow; row++ {
		buf.WriteString(FormatCUP(row, 1))
		buf.WriteString("\x1b[2K")
	}
	buf.WriteString(FormatCUP(cursorRow, cursorCol))
	buf.WriteString("\x1b[1K")
	buf.WriteString(FormatCUP(cursorRow, cursorCol))
	return buf.String()
}

func (r *Rewriter) clearChildViewport() string {
	buf := bytes.NewBuffer(nil)
	cursorRow := r.tracker.Row
	cursorCol := r.tracker.Col
	childBottom := r.layout.ChildTop + r.layout.ChildRows - 1
	for row := r.layout.ChildTop; row <= childBottom; row++ {
		buf.WriteString(FormatCUP(row, 1))
		buf.WriteString("\x1b[2K")
	}
	buf.WriteString(FormatCUP(cursorRow, cursorCol))
	return buf.String()
}

func (r *Rewriter) clearFromCursorDown() string {
	cursorRow := r.tracker.Row
	cursorCol := r.tracker.Col
	childBottom := r.layout.ChildTop + r.layout.ChildRows - 1
	if cursorRow >= childBottom {
		return "\x1b[0K"
	}
	buf := bytes.NewBuffer(nil)
	buf.WriteString("\x1b[0K")
	for row := cursorRow + 1; row <= childBottom; row++ {
		buf.WriteString(FormatCUP(row, 1))
		buf.WriteString("\x1b[2K")
	}
	buf.WriteString(FormatCUP(cursorRow, cursorCol))
	return buf.String()
}
