package snippets

import (
	"math/rand"
	"slices"
	"testing"
)

func TestNextAvoidsRecentTermsUntilCycleReset(t *testing.T) {
	t.Parallel()

	store := &Store{
		keys: []string{"a", "b", "c"},
		snippets: map[string]Item{
			"a": {Term: "a", Definition: "A"},
			"b": {Term: "b", Definition: "B"},
			"c": {Term: "c", Definition: "C"},
		},
		recentSet:  make(map[string]struct{}),
		maxRecent:  3,
		randomizer: rand.New(rand.NewSource(7)),
	}

	picks := make([]string, 0, 3)
	for range 3 {
		picks = append(picks, store.Next().Term)
	}

	if picks[0] == picks[1] || picks[0] == picks[2] || picks[1] == picks[2] {
		t.Fatalf("expected first cycle picks to be unique, got %v", picks)
	}
}

func TestRecentSetTracksRecentSlice(t *testing.T) {
	t.Parallel()

	store := &Store{
		keys: []string{"a", "b", "c", "d"},
		snippets: map[string]Item{
			"a": {Term: "a", Definition: "A"},
			"b": {Term: "b", Definition: "B"},
			"c": {Term: "c", Definition: "C"},
			"d": {Term: "d", Definition: "D"},
		},
		recentSet:  make(map[string]struct{}),
		maxRecent:  2,
		randomizer: rand.New(rand.NewSource(42)),
	}

	for range 10 {
		_ = store.Next()
		if len(store.recent) > store.maxRecent {
			t.Fatalf("recent exceeded maxRecent: %d > %d", len(store.recent), store.maxRecent)
		}
		if len(store.recentSet) != len(store.recent) {
			t.Fatalf("recentSet and recent length mismatch: set=%d slice=%d", len(store.recentSet), len(store.recent))
		}
		for _, key := range store.recent {
			if _, ok := store.recentSet[key]; !ok {
				t.Fatalf("recentSet missing key %q from recent slice", key)
			}
		}
	}
}

func TestSearchMatchesKeyTermAndDefinition(t *testing.T) {
	t.Parallel()

	store := &Store{
		keys: []string{"docker", "json", "vim"},
		snippets: map[string]Item{
			"docker": {Term: "Docker", Definition: "Container tooling"},
			"json":   {Term: "JSON", Definition: "Structured text format"},
			"vim":    {Term: "Vim", Definition: "Terminal editor"},
		},
		recentSet: make(map[string]struct{}),
	}

	matches := store.Search("text")
	if len(matches) != 1 || matches[0].Key != "json" {
		t.Fatalf("expected json match for definition query, got %+v", matches)
	}

	matches = store.Search("dock")
	keys := make([]string, 0, len(matches))
	for _, result := range matches {
		keys = append(keys, result.Key)
	}
	if !slices.Equal(keys, []string{"docker"}) {
		t.Fatalf("expected docker key match, got %v", keys)
	}
}
