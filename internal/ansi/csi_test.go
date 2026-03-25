package ansi

import "testing"

func TestParseCSI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            string
		wantPrivate     string
		wantParamsCount int
		wantFirst       int
	}{
		{name: "empty", body: "", wantPrivate: "", wantParamsCount: 0},
		{name: "private only", body: "?25", wantPrivate: "?", wantParamsCount: 1, wantFirst: 25},
		{name: "normal params", body: "12;5", wantPrivate: "", wantParamsCount: 2, wantFirst: 12},
		{name: "empty param", body: "1;;3", wantPrivate: "", wantParamsCount: 3, wantFirst: 1},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			private, params := ParseCSI(test.body)
			if private != test.wantPrivate {
				t.Fatalf("private mismatch: got %q want %q", private, test.wantPrivate)
			}
			if len(params) != test.wantParamsCount {
				t.Fatalf("param count mismatch: got %d want %d", len(params), test.wantParamsCount)
			}
			if len(params) > 0 && params[0] != test.wantFirst {
				t.Fatalf("first param mismatch: got %d want %d", params[0], test.wantFirst)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value int
		low   int
		high  int
		want  int
	}{
		{value: -1, low: 1, high: 10, want: 1},
		{value: 7, low: 1, high: 10, want: 7},
		{value: 99, low: 1, high: 10, want: 10},
	}

	for _, test := range tests {
		got := Clamp(test.value, test.low, test.high)
		if got != test.want {
			t.Fatalf("Clamp(%d, %d, %d) = %d, want %d", test.value, test.low, test.high, got, test.want)
		}
	}
}
