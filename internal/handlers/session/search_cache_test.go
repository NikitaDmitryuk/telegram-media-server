package session

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/prowlarr"
)

func TestMain(m *testing.M) {
	if logutils.Log == nil {
		logutils.InitLogger("error")
	}
	os.Exit(m.Run())
}

// clearTestCache resets the global search cache between tests.
func clearTestCache() {
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	searchCache = make(map[string]*searchCacheEntry)
}

func sampleResults(n int) []prowlarr.TorrentSearchResult {
	results := make([]prowlarr.TorrentSearchResult, n)
	for i := range n {
		results[i] = prowlarr.TorrentSearchResult{
			Title: fmt.Sprintf("Movie %d", i+1),
			Size:  int64((i + 1) * 1024 * 1024 * 1024),
			Peers: (n - i) * 10,
		}
	}
	return results
}

func TestGetCachedResults_EmptyCache(t *testing.T) {
	clearTestCache()

	results, ok := getCachedResults("some query")
	if ok {
		t.Error("Expected cache miss on empty cache")
	}
	if results != nil {
		t.Errorf("Expected nil results on cache miss, got %d items", len(results))
	}
}

func TestSetAndGetCachedResults(t *testing.T) {
	clearTestCache()

	original := sampleResults(5)
	setCachedResults("Inception 2010", original)

	results, ok := getCachedResults("Inception 2010")
	if !ok {
		t.Fatal("Expected cache hit")
	}
	if len(results) != len(original) {
		t.Fatalf("Expected %d results, got %d", len(original), len(results))
	}
	for i := range original {
		if results[i].Title != original[i].Title {
			t.Errorf("Result %d: expected title %q, got %q", i, original[i].Title, results[i].Title)
		}
		if results[i].Peers != original[i].Peers {
			t.Errorf("Result %d: expected peers %d, got %d", i, original[i].Peers, results[i].Peers)
		}
	}
}

func TestCachedResults_CaseInsensitive(t *testing.T) {
	clearTestCache()

	original := sampleResults(3)
	setCachedResults("inception", original)

	// Different cases should all hit the same cache entry.
	cases := []string{"INCEPTION", "Inception", "InCePtIoN", "inception"}
	for _, q := range cases {
		results, ok := getCachedResults(q)
		if !ok {
			t.Errorf("Expected cache hit for query %q", q)
			continue
		}
		if len(results) != len(original) {
			t.Errorf("Query %q: expected %d results, got %d", q, len(original), len(results))
		}
	}
}

func TestCachedResults_WhitespaceTrimmed(t *testing.T) {
	clearTestCache()

	original := sampleResults(2)
	setCachedResults("  movie  ", original)

	results, ok := getCachedResults("movie")
	if !ok {
		t.Error("Expected cache hit after trimming whitespace")
	}
	if len(results) != len(original) {
		t.Errorf("Expected %d results, got %d", len(original), len(results))
	}

	// And the reverse: set without spaces, get with spaces.
	clearTestCache()
	setCachedResults("film", sampleResults(1))
	results, ok = getCachedResults("  film  ")
	if !ok {
		t.Error("Expected cache hit for query with extra whitespace")
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestCachedResults_TTLExpiry(t *testing.T) {
	clearTestCache()

	// Manually insert an expired entry.
	key := "expired query"
	searchCacheMu.Lock()
	searchCache[key] = &searchCacheEntry{
		results: sampleResults(3),
		created: time.Now().Add(-searchCacheTTL - time.Minute),
	}
	searchCacheMu.Unlock()

	results, ok := getCachedResults(key)
	if ok {
		t.Error("Expected cache miss for expired entry")
	}
	if results != nil {
		t.Error("Expected nil results for expired entry")
	}
}

func TestCachedResults_EvictionOnSet(t *testing.T) {
	clearTestCache()

	// Insert an expired entry manually.
	searchCacheMu.Lock()
	searchCache["old"] = &searchCacheEntry{
		results: sampleResults(1),
		created: time.Now().Add(-searchCacheTTL - time.Minute),
	}
	searchCacheMu.Unlock()

	// Setting a new entry should evict the expired one.
	setCachedResults("new query", sampleResults(2))

	searchCacheMu.Lock()
	_, oldExists := searchCache["old"]
	_, newExists := searchCache["new query"]
	searchCacheMu.Unlock()

	if oldExists {
		t.Error("Expired entry 'old' should have been evicted")
	}
	if !newExists {
		t.Error("New entry should exist in cache")
	}
}

func TestCachedResults_ReturnsCopy(t *testing.T) {
	clearTestCache()

	original := sampleResults(3)
	setCachedResults("test", original)

	results, ok := getCachedResults("test")
	if !ok {
		t.Fatal("Expected cache hit")
	}

	// Mutate the returned copy.
	results[0].Title = "MUTATED"
	_ = append(results, prowlarr.TorrentSearchResult{Title: "EXTRA"})

	// Original cache entry should be unchanged.
	cached, ok := getCachedResults("test")
	if !ok {
		t.Fatal("Expected cache hit after mutation")
	}
	if len(cached) != len(original) {
		t.Errorf("Cache should still have %d results, got %d", len(original), len(cached))
	}
	if cached[0].Title != original[0].Title {
		t.Errorf("Cache entry was mutated: expected %q, got %q", original[0].Title, cached[0].Title)
	}
}

func TestCachedResults_MultipleQueries(t *testing.T) {
	clearTestCache()

	setCachedResults("query A", sampleResults(2))
	setCachedResults("query B", sampleResults(5))
	setCachedResults("query C", sampleResults(1))

	testsData := []struct {
		query    string
		expected int
	}{
		{"query A", 2},
		{"query B", 5},
		{"query C", 1},
		{"query D", 0}, // not cached
	}

	for _, td := range testsData {
		results, ok := getCachedResults(td.query)
		if td.expected == 0 {
			if ok {
				t.Errorf("Query %q: expected cache miss", td.query)
			}
			continue
		}
		if !ok {
			t.Errorf("Query %q: expected cache hit", td.query)
			continue
		}
		if len(results) != td.expected {
			t.Errorf("Query %q: expected %d results, got %d", td.query, td.expected, len(results))
		}
	}
}

func TestCachedResults_OverwriteExisting(t *testing.T) {
	clearTestCache()

	setCachedResults("query", sampleResults(3))
	setCachedResults("query", sampleResults(7))

	results, ok := getCachedResults("query")
	if !ok {
		t.Fatal("Expected cache hit")
	}
	if len(results) != 7 {
		t.Errorf("Expected 7 results after overwrite, got %d", len(results))
	}
}

func TestCachedResults_ConcurrentAccess(_ *testing.T) {
	clearTestCache()

	const numGoroutines = 20
	const numOperations = 500
	var wg sync.WaitGroup

	// Concurrent writers.
	for i := range numGoroutines {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := range numOperations {
				query := fmt.Sprintf("worker_%d_query_%d", workerID, j%10)
				setCachedResults(query, sampleResults(3))
			}
		}(i)
	}

	// Concurrent readers.
	for i := range numGoroutines {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := range numOperations {
				query := fmt.Sprintf("worker_%d_query_%d", workerID, j%10)
				getCachedResults(query)
			}
		}(i)
	}

	wg.Wait()
	// No race/deadlock = pass.
}

func TestCachedResults_EmptyResults(t *testing.T) {
	clearTestCache()

	// An empty result slice should still be cached and returned.
	setCachedResults("nothing", []prowlarr.TorrentSearchResult{})

	results, ok := getCachedResults("nothing")
	if !ok {
		t.Error("Expected cache hit for empty results")
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestCachedResults_FreshEntryNotExpired(t *testing.T) {
	clearTestCache()

	// Insert an entry that is almost expired but not quite.
	key := "almost expired"
	searchCacheMu.Lock()
	searchCache[key] = &searchCacheEntry{
		results: sampleResults(2),
		created: time.Now().Add(-searchCacheTTL + time.Minute),
	}
	searchCacheMu.Unlock()

	results, ok := getCachedResults(key)
	if !ok {
		t.Error("Entry should not be expired yet")
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}
