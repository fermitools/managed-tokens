// COPYRIGHT 2026 FERMI NATIONAL ACCELERATOR LABORATORY
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
//
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package worker

import "sync"

// CachedTokenStorerAndGetter is a TokenStorerAndGetter that also caches service/credd combinations
type CachedTokenStorerAndGetter interface {
	TokenStorerAndGetter
	storeInCache(serviceName string)
	hasInCache(serviceName string) bool
}

// creddServiceCache is a cache that keeps track of which service/credd combinations have already had tokens stored for them
// Since this cache can be shared, the provided store and has methods use the mutex to protect access to the cache map
type creddServiceCache struct {
	cache map[string]map[string]struct{} // {credd: {serviceName: struct{}{}}}
	mux   sync.Mutex
}

func newCreddServiceCache() creddServiceCache {
	return creddServiceCache{
		cache: make(map[string]map[string]struct{}),
	}
}

func (c *creddServiceCache) store(credd string, serviceName string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	if _, ok := c.cache[credd]; !ok {
		c.cache[credd] = make(map[string]struct{})
	}
	c.cache[credd][serviceName] = struct{}{}
}

func (c *creddServiceCache) has(credd string, serviceName string) bool {
	c.mux.Lock()
	defer c.mux.Unlock()

	if _, ok := c.cache[credd]; !ok {
		return false
	}
	if _, ok := c.cache[credd][serviceName]; !ok {
		return false
	}
	return true
}

// cachedTokenStorerAndGetter is a struct that wraps a TokenStorerAndGetter and
// adds caching functionality to keep track of which service's tokens have been stored in a
// given credd during the current worker run.
type cachedTokenStorerAndGetter struct {
	TokenStorerAndGetter
	cache *creddServiceCache
}

// newCachedTokenStorerAndGetter returns a new cachedTokenStorerAndGetter that wraps a TokenStorerAndGetter and
// prepopulates the internal cache with an optional existing cache.
func newCachedTokenStorerAndGetter(t TokenStorerAndGetter, currentCache *creddServiceCache) cachedTokenStorerAndGetter {
	if currentCache == nil {
		c := newCreddServiceCache()
		currentCache = &c
	}

	currentCache.mux.Lock()
	defer currentCache.mux.Unlock()
	if _, ok := ((*currentCache).cache)[t.GetCredd()]; !ok {
		(*currentCache).cache[t.GetCredd()] = make(map[string]struct{})
	}

	return cachedTokenStorerAndGetter{
		TokenStorerAndGetter: t,
		cache:                currentCache,
	}
}

// storeInCache adds the serviceName and credd combination to the cachedTokenStorerAndGetter cache
func (c *cachedTokenStorerAndGetter) storeInCache(serviceName string) {
	c.cache.store(c.GetCredd(), serviceName)
}

// hasInCache checks whether the serviceName and credd combination is in the cachedTokenStorerAndGetter cache
func (c *cachedTokenStorerAndGetter) hasInCache(serviceName string) bool {
	return c.cache.has(c.GetCredd(), serviceName)
}

// noOpCachedTokenStorerAndGetter is a struct that implements the CachedTokenStorerAndGetter interface
// but does not actually cache anything.
type noOpCachedTokenStorerAndGetter struct {
	TokenStorerAndGetter
}

func (n *noOpCachedTokenStorerAndGetter) storeInCache(serviceName string)    {} // No op
func (n *noOpCachedTokenStorerAndGetter) hasInCache(serviceName string) bool { return false }
