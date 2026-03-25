package ansi

import "testing"

func TestTrackerPrintableWrapsAtRightMargin(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(5, 4, 1, 5)
	tracker.SetCursor(1, 4)
	tracker.Printable(1)

	if tracker.Row != 2 || tracker.Col != 1 {
		t.Fatalf("expected wrap to row=2 col=1, got row=%d col=%d", tracker.Row, tracker.Col)
	}
}

func TestTrackerPrintableRespectsScrollRegionOnWrap(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(5, 4, 2, 3)
	tracker.SetCursor(3, 3)
	tracker.Printable(2)

	if tracker.Row != 3 || tracker.Col != 1 {
		t.Fatalf("expected wrap to stay within scroll region at row=3 col=1, got row=%d col=%d", tracker.Row, tracker.Col)
	}
}
