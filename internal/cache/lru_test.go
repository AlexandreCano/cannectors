package cache

import (
	"sync"
	"testing"
	"time"
)

func TestNewLRUCache(t *testing.T) {
	t.Run("creates cache with default values for zero maxSize and TTL", func(t *testing.T) {
		cache := NewLRUCache(0, 0)
		if cache.maxSize != 1000 {
			t.Errorf("expected default maxSize 1000, got %d", cache.maxSize)
		}
		if cache.defaultTTL != 5*time.Minute {
			t.Errorf("expected default TTL 5m, got %v", cache.defaultTTL)
		}
	})

	t.Run("creates cache with custom values", func(t *testing.T) {
		cache := NewLRUCache(500, 10*time.Second)
		if cache.maxSize != 500 {
			t.Errorf("expected maxSize 500, got %d", cache.maxSize)
		}
		if cache.defaultTTL != 10*time.Second {
			t.Errorf("expected TTL 10s, got %v", cache.defaultTTL)
		}
	})
}

func TestLRUCache_GetSet(t *testing.T) {
	t.Run("returns false for missing key", func(t *testing.T) {
		cache := NewLRUCache(10, time.Minute)
		_, found := cache.Get("nonexistent")
		if found {
			t.Error("expected false for missing key")
		}
	})

	t.Run("returns value for existing key", func(t *testing.T) {
		cache := NewLRUCache(10, time.Minute)
		cache.Set("key1", "value1", 0)

		value, found := cache.Get("key1")
		if !found {
			t.Fatal("expected to find key1")
		}
		if value != "value1" {
			t.Errorf("expected value1, got %v", value)
		}
	})

	t.Run("stores complex values", func(t *testing.T) {
		cache := NewLRUCache(10, time.Minute)
		data := map[string]any{
			"name":   "test",
			"count":  42,
			"nested": map[string]any{"key": "val"},
		}
		cache.Set("complex", data, 0)

		value, found := cache.Get("complex")
		if !found {
			t.Fatal("expected to find complex key")
		}

		result, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("expected map, got %T", value)
		}
		if result["name"] != "test" {
			t.Errorf("expected name=test, got %v", result["name"])
		}
	})
}

func TestLRUCache_TTLExpiration(t *testing.T) {
	t.Run("returns false for expired key", func(t *testing.T) {
		cache := NewLRUCache(10, time.Minute)
		cache.Set("key1", "value1", 50*time.Millisecond)

		// Should find it immediately
		_, found := cache.Get("key1")
		if !found {
			t.Fatal("expected to find key1 before expiration")
		}

		// Wait for expiration
		time.Sleep(60 * time.Millisecond)

		_, found = cache.Get("key1")
		if found {
			t.Error("expected key1 to be expired")
		}
	})

	t.Run("uses default TTL when 0 is provided", func(t *testing.T) {
		cache := NewLRUCache(10, 50*time.Millisecond)
		cache.Set("key1", "value1", 0)

		_, found := cache.Get("key1")
		if !found {
			t.Fatal("expected to find key1 before expiration")
		}

		time.Sleep(60 * time.Millisecond)

		_, found = cache.Get("key1")
		if found {
			t.Error("expected key1 to be expired with default TTL")
		}
	})
}

func TestLRUCache_LRUEviction(t *testing.T) {
	t.Run("evicts LRU entry when at capacity", func(t *testing.T) {
		cache := NewLRUCache(3, time.Minute)

		cache.Set("key1", "value1", 0)
		time.Sleep(5 * time.Millisecond)
		cache.Set("key2", "value2", 0)
		time.Sleep(5 * time.Millisecond)
		cache.Set("key3", "value3", 0)

		// Access key1 to make it more recently used
		cache.Get("key1")
		time.Sleep(5 * time.Millisecond)

		// Add new key - should evict key2 (LRU)
		cache.Set("key4", "value4", 0)

		if cache.Size() != 3 {
			t.Errorf("expected size 3, got %d", cache.Size())
		}

		// key1 should still exist (was accessed)
		_, found := cache.Get("key1")
		if !found {
			t.Error("key1 should exist (was recently accessed)")
		}

		// key2 should be evicted (LRU)
		_, found = cache.Get("key2")
		if found {
			t.Error("key2 should have been evicted (LRU)")
		}

		// key3 and key4 should exist
		_, found = cache.Get("key3")
		if !found {
			t.Error("key3 should exist")
		}
		_, found = cache.Get("key4")
		if !found {
			t.Error("key4 should exist")
		}
	})

	t.Run("updates existing key without eviction", func(t *testing.T) {
		cache := NewLRUCache(2, time.Minute)

		cache.Set("key1", "value1", 0)
		cache.Set("key2", "value2", 0)

		// Update key1 - should not evict anything
		cache.Set("key1", "updated", 0)

		if cache.Size() != 2 {
			t.Errorf("expected size 2, got %d", cache.Size())
		}

		value, found := cache.Get("key1")
		if !found {
			t.Fatal("key1 should exist")
		}
		if value != "updated" {
			t.Errorf("expected updated, got %v", value)
		}
	})
}

func TestLRUCache_Delete(t *testing.T) {
	cache := NewLRUCache(10, time.Minute)
	cache.Set("key1", "value1", 0)

	cache.Delete("key1")

	_, found := cache.Get("key1")
	if found {
		t.Error("expected key1 to be deleted")
	}

	// Delete non-existent key should not panic
	cache.Delete("nonexistent")
}

func TestLRUCache_Clear(t *testing.T) {
	cache := NewLRUCache(10, time.Minute)
	cache.Set("key1", "value1", 0)
	cache.Set("key2", "value2", 0)
	cache.Set("key3", "value3", 0)

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}

	_, found := cache.Get("key1")
	if found {
		t.Error("expected cache to be empty after clear")
	}
}

func TestLRUCache_Size(t *testing.T) {
	cache := NewLRUCache(10, time.Minute)

	if cache.Size() != 0 {
		t.Errorf("expected initial size 0, got %d", cache.Size())
	}

	cache.Set("key1", "value1", 0)
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}

	cache.Set("key2", "value2", 0)
	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	cache.Delete("key1")
	if cache.Size() != 1 {
		t.Errorf("expected size 1 after delete, got %d", cache.Size())
	}
}

func TestLRUCache_Stats(t *testing.T) {
	cache := NewLRUCache(2, time.Minute)

	// Initial stats should be zero
	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Error("expected initial stats to be zero")
	}

	// Set and get - should be a hit
	cache.Set("key1", "value1", 0)
	cache.Get("key1")
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}

	// Get missing key - should be a miss
	cache.Get("nonexistent")
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}

	// Trigger eviction
	cache.Set("key2", "value2", 0)
	cache.Set("key3", "value3", 0) // Should evict key1
	stats = cache.Stats()
	if stats.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evictions)
	}

	// Get expired key - should be a miss
	cache.Set("expiring", "value", 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	cache.Get("expiring")
	stats = cache.Stats()
	if stats.Misses != 2 {
		t.Errorf("expected 2 misses (including expired), got %d", stats.Misses)
	}
}

func TestLRUCache_CleanExpired(t *testing.T) {
	cache := NewLRUCache(10, time.Minute)

	cache.Set("expire1", "value1", 20*time.Millisecond)
	cache.Set("expire2", "value2", 20*time.Millisecond)
	cache.Set("keep", "value3", time.Minute)

	// Wait for expiration
	time.Sleep(30 * time.Millisecond)

	cleaned := cache.CleanExpired()
	if cleaned != 2 {
		t.Errorf("expected 2 cleaned entries, got %d", cleaned)
	}

	if cache.Size() != 1 {
		t.Errorf("expected size 1 after clean, got %d", cache.Size())
	}

	_, found := cache.Get("keep")
	if !found {
		t.Error("keep should still exist")
	}
}

func TestLRUCache_ConcurrentAccess(t *testing.T) {
	cache := NewLRUCache(100, time.Minute)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				cache.Set("key", n*100+j, 0)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				cache.Get("key")
			}
		}()
	}

	wg.Wait()

	// Should not panic and cache should be in valid state
	if cache.Size() < 0 || cache.Size() > 100 {
		t.Errorf("unexpected cache size: %d", cache.Size())
	}
}

func TestLRUCache_NilValue(t *testing.T) {
	cache := NewLRUCache(10, time.Minute)
	cache.Set("nilkey", nil, 0)

	value, found := cache.Get("nilkey")
	if !found {
		t.Error("expected to find nilkey")
	}
	if value != nil {
		t.Errorf("expected nil value, got %v", value)
	}
}
