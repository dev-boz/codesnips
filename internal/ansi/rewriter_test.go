package ansi

import (
	"bytes"
	"testing"
)

func TestRewriterGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		layout   Layout
		input    []byte
		wantOut  []byte
		wantRow  int
		wantCol  int
		startRow int
		startCol int
	}{
		{
			name:     "CUP is remapped into child viewport",
			layout:   Layout{ChildTop: 3, ChildRows: 10, Cols: 80},
			input:    []byte("\x1b[2;5H"),
			wantOut:  []byte("\x1b[4;5H"),
			wantRow:  4,
			wantCol:  5,
			startRow: 3,
			startCol: 1,
		},
		{
			name:     "scroll region is remapped",
			layout:   Layout{ChildTop: 3, ChildRows: 10, Cols: 80},
			input:    []byte("\x1b[2;4r"),
			wantOut:  []byte("\x1b[4;6r"),
			wantRow:  4,
			wantCol:  1,
			startRow: 3,
			startCol: 1,
		},
		{
			name:     "plain UTF-8 passes through unchanged",
			layout:   Layout{ChildTop: 1, ChildRows: 10, Cols: 80},
			input:    []byte("naïve"),
			wantOut:  []byte("naïve"),
			wantRow:  1,
			wantCol:  6,
			startRow: 1,
			startCol: 1,
		},
		{
			name:     "wide UTF-8 advances by display width",
			layout:   Layout{ChildTop: 1, ChildRows: 10, Cols: 80},
			input:    []byte("界"),
			wantOut:  []byte("界"),
			wantRow:  1,
			wantCol:  3,
			startRow: 1,
			startCol: 1,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tracker := NewTracker(24, test.layout.Cols, test.layout.ChildTop, test.layout.ChildTop+test.layout.ChildRows-1)
			tracker.SetCursor(test.startRow, test.startCol)
			rewriter := NewRewriter(tracker, test.layout, Callbacks{})

			got := rewriter.Feed(test.input)
			if !bytes.Equal(got, test.wantOut) {
				t.Fatalf("output mismatch:\n got: %q\nwant: %q", string(got), string(test.wantOut))
			}
			if tracker.Row != test.wantRow || tracker.Col != test.wantCol {
				t.Fatalf("cursor mismatch: got row=%d col=%d want row=%d col=%d", tracker.Row, tracker.Col, test.wantRow, test.wantCol)
			}
		})
	}
}

func TestRewriterTracksUTF8AcrossChunks(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(24, 80, 1, 24)
	rewriter := NewRewriter(tracker, Layout{ChildTop: 1, ChildRows: 24, Cols: 80}, Callbacks{})

	input := []byte("€")
	first := rewriter.Feed(input[:1])
	second := rewriter.Feed(input[1:])

	if tracker.Col != 2 {
		t.Fatalf("expected one printable column advance for one UTF-8 rune, got col=%d", tracker.Col)
	}
	if !bytes.Equal(append(first, second...), input) {
		t.Fatalf("output bytes changed across chunked UTF-8 input")
	}
}

func TestRewriterTracksWideUTF8AcrossChunks(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(24, 80, 1, 24)
	rewriter := NewRewriter(tracker, Layout{ChildTop: 1, ChildRows: 24, Cols: 80}, Callbacks{})

	input := []byte("界")
	first := rewriter.Feed(input[:2])
	second := rewriter.Feed(input[2:])

	if tracker.Col != 3 {
		t.Fatalf("expected one wide rune to advance two columns, got col=%d", tracker.Col)
	}
	if !bytes.Equal(append(first, second...), input) {
		t.Fatalf("output bytes changed across chunked wide UTF-8 input")
	}
}
