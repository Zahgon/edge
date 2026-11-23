package edge

import "github.com/edge-js/edge-go/js"

// CacheManager caches compiled templates in memory, keyed by their absolute
// path. When it is disabled, Get finds nothing and Set and Delete do nothing;
// Has still reports what the store holds.
type CacheManager struct {
	Enabled    bool
	cacheStore map[string]*js.Function
}

// newCacheManager returns an empty cache manager.
func newCacheManager(enabled bool) *CacheManager {
	return &CacheManager{Enabled: enabled}
}

// Has reports whether a template has been cached.
func (m *CacheManager) Has(absPath string) bool {
	_, ok := m.cacheStore[absPath]
	return ok
}

// Get returns the cached template, or nil when caching is disabled or the
// template is not cached.
func (m *CacheManager) Get(absPath string) *js.Function {
	if !m.Enabled {
		return nil
	}
	return m.cacheStore[absPath]
}

// Set caches a compiled template.
func (m *CacheManager) Set(absPath string, compiled *js.Function) {
	if !m.Enabled {
		return
	}
	if m.cacheStore == nil {
		m.cacheStore = map[string]*js.Function{}
	}
	m.cacheStore[absPath] = compiled
}

// Delete removes a template from the cache.
func (m *CacheManager) Delete(absPath string) {
	if !m.Enabled {
		return
	}
	delete(m.cacheStore, absPath)
}
