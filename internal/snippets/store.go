package snippets

import (
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"
)

type Item struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

type SearchResult struct {
	Key  string
	Item Item
}

type Store struct {
	keys       []string
	snippets   map[string]Item
	recent     []string
	recentSet  map[string]struct{}
	maxRecent  int
	randomizer *rand.Rand
}

func Load(path string) (*Store, error) {
	var (
		data []byte
		err  error
	)

	if path == "" {
		data = embeddedDefaults
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	items := make(map[string]Item)
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("snippets file is empty")
	}

	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return &Store{
		keys:       keys,
		snippets:   items,
		recentSet:  make(map[string]struct{}),
		maxRecent:  min(10, len(keys)),
		randomizer: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (s *Store) Terms() []string {
	terms := make([]string, len(s.keys))
	copy(terms, s.keys)
	return terms
}

func (s *Store) Get(key string) (Item, bool) {
	item, ok := s.snippets[key]
	if !ok {
		return Item{}, false
	}
	return normalizeItem(key, item), true
}

func (s *Store) Search(query string) []SearchResult {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return nil
	}

	results := make([]SearchResult, 0)
	for _, key := range s.keys {
		item := s.snippets[key]
		if strings.Contains(strings.ToLower(key), queryLower) ||
			strings.Contains(strings.ToLower(item.Term), queryLower) ||
			strings.Contains(strings.ToLower(item.Definition), queryLower) {
			results = append(results, SearchResult{
				Key:  key,
				Item: normalizeItem(key, item),
			})
		}
	}
	return results
}

func (s *Store) Next() Item {
	candidates := make([]string, 0, len(s.keys))
	for _, key := range s.keys {
		if _, seen := s.recentSet[key]; !seen {
			candidates = append(candidates, key)
		}
	}

	if len(candidates) == 0 {
		s.recent = s.recent[:0]
		clear(s.recentSet)
		candidates = append(candidates, s.keys...)
	}

	key := candidates[s.randomizer.Intn(len(candidates))]
	s.recent = append(s.recent, key)
	s.recentSet[key] = struct{}{}
	if len(s.recent) > s.maxRecent {
		oldest := s.recent[0]
		s.recent = s.recent[1:]
		delete(s.recentSet, oldest)
	}

	return normalizeItem(key, s.snippets[key])
}

func (s *Store) Intn(bound int) int {
	return s.randomizer.Intn(bound)
}

func normalizeItem(key string, item Item) Item {
	if item.Term == "" {
		item.Term = key
	}
	if item.Definition == "" {
		item.Definition = "No definition available."
	}
	return item
}
