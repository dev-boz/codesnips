package ansi

type Tracker struct {
	Rows         int
	Cols         int
	Row          int
	Col          int
	SavedRow     int
	SavedCol     int
	ScrollTop    int
	ScrollBottom int
}

func NewTracker(rows, cols, scrollTop, scrollBottom int) *Tracker {
	return &Tracker{
		Rows:         rows,
		Cols:         cols,
		Row:          scrollTop,
		Col:          1,
		SavedRow:     scrollTop,
		SavedCol:     1,
		ScrollTop:    scrollTop,
		ScrollBottom: scrollBottom,
	}
}

func (t *Tracker) Resize(rows, cols, scrollTop, scrollBottom int) {
	t.Rows = rows
	t.Cols = cols
	t.ScrollTop = scrollTop
	t.ScrollBottom = scrollBottom
	t.Row = Clamp(t.Row, scrollTop, scrollBottom)
	t.Col = Clamp(t.Col, 1, max(1, cols))
	t.SavedRow = Clamp(t.SavedRow, scrollTop, scrollBottom)
	t.SavedCol = Clamp(t.SavedCol, 1, max(1, cols))
}

func (t *Tracker) Save() {
	t.SavedRow = t.Row
	t.SavedCol = t.Col
}

func (t *Tracker) Restore() {
	t.Row = Clamp(t.SavedRow, 1, t.Rows)
	t.Col = Clamp(t.SavedCol, 1, max(1, t.Cols))
}

func (t *Tracker) SetCursor(row, col int) {
	t.Row = Clamp(row, 1, t.Rows)
	t.Col = Clamp(col, 1, max(1, t.Cols))
}

func (t *Tracker) SetScrollRegion(top, bottom int) {
	top = Clamp(top, 1, t.Rows)
	bottom = Clamp(bottom, top, t.Rows)
	t.ScrollTop = top
	t.ScrollBottom = bottom
	t.Row = top
	t.Col = 1
}

func (t *Tracker) SetCol(col int) {
	t.Col = Clamp(col, 1, max(1, t.Cols))
}

func (t *Tracker) MoveDown(lines int) {
	if lines <= 0 {
		return
	}
	t.Row += lines
	if t.Row > t.ScrollBottom {
		t.Row = t.ScrollBottom
	}
}

func (t *Tracker) MoveUp(lines int) {
	if lines <= 0 {
		return
	}
	t.Row -= lines
	if t.Row < t.ScrollTop {
		t.Row = t.ScrollTop
	}
}

func (t *Tracker) MoveRight(cols int) {
	if cols <= 0 {
		return
	}
	t.Col += cols
	if t.Col > t.Cols {
		t.Col = t.Cols
	}
}

func (t *Tracker) MoveLeft(cols int) {
	if cols <= 0 {
		return
	}
	t.Col -= cols
	if t.Col < 1 {
		t.Col = 1
	}
}

func (t *Tracker) Printable(width int) {
	if width <= 0 {
		width = 1
	}
	cols := max(1, t.Cols)
	total := t.Col - 1 + width
	wraps := total / cols
	t.Col = total%cols + 1
	for i := 0; i < wraps; i++ {
		t.LineFeed()
	}
}

func (t *Tracker) LineFeed() {
	if t.Row >= t.ScrollBottom {
		t.Row = t.ScrollBottom
		return
	}
	t.Row++
}

func (t *Tracker) ReverseIndex() {
	if t.Row <= t.ScrollTop {
		t.Row = t.ScrollTop
		return
	}
	t.Row--
}

func Clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
