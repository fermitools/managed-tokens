package worker

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoOpCachedTokenStorerAndGetter(t *testing.T) {
	// If we run store and has methods on noOpCachedTokenStorerAndGetter, they should be no-ops, and nothing should get stored
	testService := "test_service"
	testCredd := "test_credd"
	testVaultServer := "test_vault_server"
	f := &fakeTokenStorerAndGetter{t: t, credd: testCredd, vaultServer: testVaultServer, shouldFail: false}
	n := &noOpCachedTokenStorerAndGetter{TokenStorerAndGetter: f}

	n.storeInCache(testService)
	assert.False(t, n.hasInCache(testService))
}
func TestNewCreddServiceCache(t *testing.T) {
	t.Run("creates cache with empty map", func(t *testing.T) {
		cache := newCreddServiceCache()
		assert.NotNil(t, cache.cache)
		assert.Equal(t, len(cache.cache), 0)
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		cache := newCreddServiceCache()
		var wg sync.WaitGroup

		for i := range 10 {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				cache.mux.Lock()
				defer cache.mux.Unlock()
				credd := "credd_" + strconv.Itoa(index)
				service := "service_" + strconv.Itoa(index)
				cache.cache[credd] = make(map[string]struct{})
				cache.cache[credd][service] = struct{}{}
			}(i)
		}
		wg.Wait()

		assert.Equal(t, len(cache.cache), 10)
	})
}
func TestCreddServiceCacheStore(t *testing.T) {
	t.Run("store new credd and service", func(t *testing.T) {
		cache := newCreddServiceCache()
		cache.store("test_credd", "test_service")
		_, ok := cache.cache["test_credd"]["test_service"]
		assert.True(t, ok)
	})

	t.Run("store multiple services in same credd", func(t *testing.T) {
		cache := newCreddServiceCache()
		cache.store("test_credd", "service1")
		cache.store("test_credd", "service2")

		_, ok1 := cache.cache["test_credd"]["service1"]
		_, ok2 := cache.cache["test_credd"]["service2"]
		assert.True(t, ok1)
		assert.True(t, ok2)
	})

	t.Run("store same service multiple times is idempotent", func(t *testing.T) {
		cache := newCreddServiceCache()
		cache.store("test_credd", "test_service")
		cache.store("test_credd", "test_service")

		_, ok := cache.cache["test_credd"]["test_service"]
		assert.True(t, ok)
	})

	t.Run("store in multiple credds concurrently", func(t *testing.T) {
		cache := newCreddServiceCache()
		var wg sync.WaitGroup

		for i := range 5 {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				credd := "credd_" + strconv.Itoa(index)
				service := "service_" + strconv.Itoa(index)
				cache.store(credd, service)
			}(i)
		}
		wg.Wait()

		assert.Equal(t, len(cache.cache), 5)
	})
}

func TestCreddServiceCacheHas(t *testing.T) {
	tests := []struct {
		name     string
		credd    string
		service  string
		setup    func(*creddServiceCache)
		expected bool
	}{
		{
			name:     "has returns false for empty cache",
			credd:    "test_credd",
			service:  "test_service",
			setup:    func(c *creddServiceCache) {},
			expected: false,
		},
		{
			name:    "has returns true for existing entry",
			credd:   "test_credd",
			service: "test_service",
			setup: func(c *creddServiceCache) {
				c.mux.Lock()
				defer c.mux.Unlock()
				c.cache["test_credd"] = make(map[string]struct{})
				c.cache["test_credd"]["test_service"] = struct{}{}
			},
			expected: true,
		},
		{
			name:    "has returns false for non-existing service",
			credd:   "test_credd",
			service: "service2",
			setup: func(c *creddServiceCache) {
				c.mux.Lock()
				defer c.mux.Unlock()
				c.cache["test_credd"] = make(map[string]struct{})
				c.cache["test_credd"]["test_service"] = struct{}{}
			},
			expected: false,
		},
		{
			name:    "has returns false for non-existing credd",
			credd:   "credd2",
			service: "service1",
			setup: func(c *creddServiceCache) {
				c.mux.Lock()
				defer c.mux.Unlock()
				c.cache["test_credd"] = make(map[string]struct{})
				c.cache["test_credd"]["test_service"] = struct{}{}
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newCreddServiceCache()
			tt.setup(&cache)
			assert.Equal(t, tt.expected, cache.has(tt.credd, tt.service))
		})
	}
}
func TestNewCachedTokenStorerAndGetter(t *testing.T) {
	t.Run("creates new cache when currentCache is nil", func(t *testing.T) {
		f := &fakeTokenStorerAndGetter{t: t, credd: "test_credd", vaultServer: "test_vault_server", shouldFail: false}
		result := newCachedTokenStorerAndGetter(f, nil)

		assert.NotNil(t, result.cache)
		assert.NotNil(t, result.cache.cache)
		assert.Equal(t, result.TokenStorerAndGetter, f)
	})

	t.Run("uses provided cache when not nil", func(t *testing.T) {
		f := &fakeTokenStorerAndGetter{t: t, credd: "test_credd", vaultServer: "test_vault_server", shouldFail: false}
		existingCache := newCreddServiceCache()
		result := newCachedTokenStorerAndGetter(f, &existingCache)

		assert.Equal(t, result.cache, &existingCache)
		assert.Equal(t, result.TokenStorerAndGetter, f)
	})

	t.Run("initializes credd map in cache if not present", func(t *testing.T) {
		f := &fakeTokenStorerAndGetter{t: t, credd: "test_credd", vaultServer: "test_vault_server", shouldFail: false}
		cache := newCreddServiceCache()
		result := newCachedTokenStorerAndGetter(f, &cache)

		_, ok := result.cache.cache["test_credd"]
		assert.True(t, ok)
	})

	t.Run("preserves existing credd map in cache", func(t *testing.T) {
		f := &fakeTokenStorerAndGetter{t: t, credd: "test_credd", vaultServer: "test_vault_server", shouldFail: false}
		cache := newCreddServiceCache()
		cache.cache["test_credd"] = make(map[string]struct{})
		cache.cache["test_credd"]["existing_service"] = struct{}{}

		result := newCachedTokenStorerAndGetter(f, &cache)

		_, ok := result.cache.cache["test_credd"]["existing_service"]
		assert.True(t, ok)
	})

	t.Run("wraps TokenStorerAndGetter correctly", func(t *testing.T) {
		f := &fakeTokenStorerAndGetter{t: t, credd: "test_credd", vaultServer: "test_vault_server", shouldFail: false}
		result := newCachedTokenStorerAndGetter(f, nil)

		assert.Equal(t, result.GetCredd(), "test_credd")
		assert.Equal(t, result.GetVaultServer(), "test_vault_server")
	})
}
