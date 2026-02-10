package worker

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fermitools/managed-tokens/internal/service"
)

func TestStoreAndGetTokenWorker(t *testing.T) {
	ctx := context.Background()
	schedd := "test_schedd"
	vaultServer := "test_vault_server"

	// bad case
	t.Run("store and get token worker fails", func(t *testing.T) {
		t.Parallel()
		chans := NewChannelsForWorkers(1)
		t.Cleanup(func() {
			select {
			case _, ok := <-chans.GetSuccessChan():
				if ok {
					chans.closeWorkerSendChans()
				}
			default:
			}
		})

		f := &fakeTokenStorerAndGetter{t: t, credd: schedd, vaultServer: vaultServer, shouldFail: true}

		s := service.NewService("testbad_service")
		sc, _ := NewConfig(s, SetSchedds([]string{"bad_schedd"}), SetAlternateTokenStorerAndGetterOption(StoreAndGetToken, f))
		chans.GetServiceConfigChan() <- sc
		close(chans.GetServiceConfigChan())
		go storeAndGetTokenWorker(ctx, chans)
		select {
		case n := <-chans.GetNotificationsChan():
			assert.NotNil(t, n, "Expected notification on NotificationsChan, got nil")
			assert.Contains(t, n.GetMessage(), "Could not store and get vault tokens")
			assert.Equal(t, n.GetService(), "testbad_service", "Expected service name to be 'testbad_service'")
		case <-time.After(10 * time.Second):
			t.Error("Expected notification on NotificationsChan, got none after 10 second timeout")
		}

		select {
		case s := <-chans.GetSuccessChan():
			assert.Equal(t, s.GetService().Name(), "testbad_service", "Expected service name to be 'testbad_service'")
			assert.False(t, s.GetSuccess(), "Expected success=false on getTokenSuccess, got success=true")
		case <-time.After(10 * time.Second):
			t.Error("Expected getTokenSuccess on SuccessChan, got none after 10 second timeout")
		}
	})

	// Good case
	t.Run("store and get token worker succeeds", func(t *testing.T) {
		t.Parallel()

		f := &fakeTokenStorerAndGetter{t: t, credd: schedd, vaultServer: vaultServer, shouldFail: false}
		s2 := service.NewService("testgood_service")
		chans2 := NewChannelsForWorkers(1)
		t.Cleanup(func() {
			select {
			case _, ok := <-chans2.GetSuccessChan():
				if ok {
					chans2.closeWorkerSendChans()
				}
			default:
			}
		})
		sc2, _ := NewConfig(s2, SetAlternateTokenStorerAndGetterOption(StoreAndGetToken, f))
		chans2.GetServiceConfigChan() <- sc2
		close(chans2.GetServiceConfigChan())
		go storeAndGetTokenWorker(ctx, chans2)
		select {
		case n, ok := <-chans2.GetNotificationsChan():
			if ok || n != nil {
				t.Error("Channel should have been closed with no values received")
			}
		case <-time.After(10 * time.Second):
			t.Error("Expected closed NotificationsChan, got none after 10 second timeout")
		}

		select {
		case s := <-chans2.GetSuccessChan():
			assert.Equal(t, s.GetService().Name(), "testgood_service", "Expected service name to be 'testgood_service'")
			assert.True(t, s.GetSuccess(), "Expected success=true on storeAndGetTokenSuccess, got success=false")
		case <-time.After(10 * time.Second):
			t.Error("Expected storeAndGetTokenSuccess on SuccessChan, got none after 10 second timeout")

		}
	})
}

type fakeTokenStorerAndGetter struct {
	t           *testing.T
	credd       string
	vaultServer string
	shouldFail  bool
}

func (f *fakeTokenStorerAndGetter) GetAndStoreToken(ctx context.Context, serviceName string, interactive bool) error {
	f.t.Log("Using fakeTokenStorerAndGetter")
	if f.shouldFail {
		return errors.New("simulated error")
	}
	return nil
}

func (f *fakeTokenStorerAndGetter) GetCredd() string       { return f.credd }
func (f *fakeTokenStorerAndGetter) GetVaultServer() string { return f.vaultServer }

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
