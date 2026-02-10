package worker

import (
	"context"
	"errors"
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

func TestNewCachedTokenStorerAndGetter(t *testing.T) {
	testService := "test_service"
	testCredd := "test_credd"
	testVaultServer := "test_vault_server"
	f := &fakeTokenStorerAndGetter{t: t, credd: testCredd, vaultServer: testVaultServer, shouldFail: false}

	// Are the credd and vault server set properly?
	checkCreddAndVaultServer := func(t *testing.T, c *cachedTokenStorerAndGetter) {
		assert.Equal(t, c.GetCredd(), testCredd)
		assert.Equal(t, c.GetVaultServer(), testVaultServer)
	}

	// Check that the cache map has our test credd/test service combo stored correctly
	checkCacheForTestCreddAndTestService := func(t *testing.T, c *cachedTokenStorerAndGetter) {
		_, ok := c.cache[testCredd][testService]
		assert.True(t, ok)
	}

	// Check that before we populate the cache, it's empty, and after populating, it has the expected values
	checkInitAndPopulatedCache := func(t *testing.T, c *cachedTokenStorerAndGetter) {
		// Before storing anything, the map should be initialized with just the credd value
		_, ok := c.cache[testCredd]
		assert.True(t, ok)

		// Now, we populate the cache map
		c.cache[testCredd] = make(map[string]struct{})
		c.cache[testCredd][testService] = struct{}{}
		checkCacheForTestCreddAndTestService(t, c)
	}

	type testCase struct {
		description  string
		currentCache map[string]map[string]struct{}
		cacheTest    func(t *testing.T, c *cachedTokenStorerAndGetter)
	}

	testCases := []testCase{
		{
			description:  "nil map passed in",
			currentCache: nil,
			cacheTest:    checkInitAndPopulatedCache,
		},
		{
			description:  "empty non-nil map passed in",
			currentCache: make(map[string]map[string]struct{}),
			cacheTest:    checkInitAndPopulatedCache,
		},
		{
			description: "pre-existing cache map passed in",
			currentCache: map[string]map[string]struct{}{
				testCredd: {
					testService: struct{}{},
				},
			},
			cacheTest: checkCacheForTestCreddAndTestService,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			c := newCachedTokenStorerAndGetter(f, tc.currentCache)
			checkCreddAndVaultServer(t, &c)
			tc.cacheTest(t, &c)
		})
	}
}

func TestCachedTokenStorerAndGetterStoreCacheValue(t *testing.T) {
	testService := "test_service"
	testCredd := "test_credd"
	testVaultServer := "test_vault_server"
	f := &fakeTokenStorerAndGetter{t: t, credd: testCredd, vaultServer: testVaultServer, shouldFail: false}
	c := cachedTokenStorerAndGetter{
		TokenStorerAndGetter: f,
		cache:                make(map[string]map[string]struct{}),
	}

	// Test storing a new credd/service combo
	t.Run("store new credd/service combo", func(t *testing.T) {
		c.storeInCache(testService)
		_, ok := c.cache[testCredd][testService]
		assert.True(t, ok)
	})

	// If we have already stored this credd/service combo, storing it again should be a no-op
	t.Run("store existing credd/service combo", func(t *testing.T) {
		service := "test_service_2"
		// Pre-populate the cache
		c.cache[testCredd] = make(map[string]struct{})
		c.cache[testCredd][service] = struct{}{}

		// Now try to store same values again
		c.storeInCache(service)
		_, ok := c.cache[testCredd][service]
		assert.True(t, ok)
	})

	// Try storing two different services in 2 different credds concurrently (should be safe due to mutex)
	t.Run("concurrent store of different service combos", func(t *testing.T) {
		var wg sync.WaitGroup
		service2 := "service2"
		service3 := "service3"
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.storeInCache(service2)
		}()
		go func() {
			defer wg.Done()
			c.storeInCache(service3)
		}()
		wg.Wait()

		_, ok := c.cache[testCredd][service2]
		assert.True(t, ok)
		_, ok = c.cache[testCredd][service3]
		assert.True(t, ok)
	})
}

func TestCachedTokenStorerAndGetterHasCacheValue(t *testing.T) {
	testService := "test_service"
	testCredd := "test_credd"
	testVaultServer := "test_vault_server"
	f := &fakeTokenStorerAndGetter{t: t, credd: testCredd, vaultServer: testVaultServer, shouldFail: false}
	c := cachedTokenStorerAndGetter{
		TokenStorerAndGetter: f,
		cache: map[string]map[string]struct{}{
			testCredd: {
				testService: struct{}{},
			},
		},
	}

	// Test loading an existing credd/service combo
	t.Run("existing credd/service combo", func(t *testing.T) {
		assert.True(t, c.hasInCache(testService))
	})

	// Test hasing a non-existing service under existing credd
	t.Run("non-existing service under existing credd", func(t *testing.T) {
		assert.False(t, c.hasInCache("non_existing_service"))
	})
}

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
