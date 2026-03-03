package daemon

import (
	"container/list"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fastbrew/internal/brew"
	"fastbrew/internal/services"

	"golang.org/x/sync/singleflight"
)

const (
	installedTTL    = 5 * time.Second
	outdatedTTL     = 15 * time.Second
	searchTTL       = 5 * time.Minute
	depsTTL         = 5 * time.Minute
	leavesTTL       = 30 * time.Second
	tapInfoTTL      = 30 * time.Second
	servicesListTTL = 5 * time.Second
	metadataTTL     = 10 * time.Minute
	metadataLRUCap  = 512
)

type cacheSlice[T any] struct {
	value   []T
	expires time.Time
}

type cachePtr[T any] struct {
	value   *T
	expires time.Time
}

type ttlLRUEntry[T any] struct {
	key     string
	value   T
	expires time.Time
}

type ttlLRU[T any] struct {
	capacity int
	ttl      time.Duration

	mu    sync.Mutex
	order *list.List
	items map[string]*list.Element
}

func newTTLLRU[T any](capacity int, ttl time.Duration) *ttlLRU[T] {
	return &ttlLRU[T]{
		capacity: capacity,
		ttl:      ttl,
		order:    list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

func (l *ttlLRU[T]) Get(key string) (T, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var zero T
	elem, ok := l.items[key]
	if !ok {
		return zero, false
	}

	entry := elem.Value.(*ttlLRUEntry[T])
	if time.Now().After(entry.expires) {
		l.order.Remove(elem)
		delete(l.items, key)
		return zero, false
	}

	l.order.MoveToFront(elem)
	return entry.value, true
}

func (l *ttlLRU[T]) Put(key string, value T) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if elem, ok := l.items[key]; ok {
		entry := elem.Value.(*ttlLRUEntry[T])
		entry.value = value
		entry.expires = time.Now().Add(l.ttl)
		l.order.MoveToFront(elem)
		return
	}

	entry := &ttlLRUEntry[T]{
		key:     key,
		value:   value,
		expires: time.Now().Add(l.ttl),
	}
	elem := l.order.PushFront(entry)
	l.items[key] = elem

	for l.order.Len() > l.capacity {
		back := l.order.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*ttlLRUEntry[T])
		delete(l.items, evicted.key)
		l.order.Remove(back)
	}
}

func (l *ttlLRU[T]) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.order.Init()
	l.items = make(map[string]*list.Element, l.capacity)
}

func (l *ttlLRU[T]) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

type Cache struct {
	mu sync.RWMutex
	sf singleflight.Group

	installed cacheSlice[brew.PackageInfo]
	outdated  cacheSlice[brew.OutdatedPackage]
	leaves    cacheSlice[string]

	searchByQuery map[string]cacheSlice[brew.SearchItem]
	depsByQuery   map[string]cacheSlice[string]
	tapInfoByKey  map[string]cachePtr[brew.TapInfo]
	servicesByKey map[string]cacheSlice[services.Service]

	formulaMeta *ttlLRU[*brew.RemoteFormula]
	caskMeta    *ttlLRU[*brew.CaskMetadata]

	lastWarmup time.Time

	requestsTotal atomic.Uint64
	cacheHits     atomic.Uint64
	cacheMisses   atomic.Uint64
}

func NewCache() *Cache {
	return &Cache{
		searchByQuery: make(map[string]cacheSlice[brew.SearchItem]),
		depsByQuery:   make(map[string]cacheSlice[string]),
		tapInfoByKey:  make(map[string]cachePtr[brew.TapInfo]),
		servicesByKey: make(map[string]cacheSlice[services.Service]),
		formulaMeta:   newTTLLRU[*brew.RemoteFormula](metadataLRUCap, metadataTTL),
		caskMeta:      newTTLLRU[*brew.CaskMetadata](metadataLRUCap, metadataTTL),
	}
}

func (c *Cache) TrackRequest() {
	c.requestsTotal.Add(1)
}

func (c *Cache) loadInstalled(loader func() ([]brew.PackageInfo, error)) ([]brew.PackageInfo, error) {
	c.mu.RLock()
	if time.Now().Before(c.installed.expires) {
		items := cloneSlice(c.installed.value)
		c.mu.RUnlock()
		c.cacheHits.Add(1)
		return items, nil
	}
	c.mu.RUnlock()
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("installed", func() (interface{}, error) {
		items, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		c.mu.Lock()
		c.installed = cacheSlice[brew.PackageInfo]{
			value:   cloneSlice(items),
			expires: time.Now().Add(installedTTL),
		}
		c.mu.Unlock()
		return cloneSlice(items), nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]brew.PackageInfo), nil
}

func (c *Cache) loadOutdated(loader func() ([]brew.OutdatedPackage, error)) ([]brew.OutdatedPackage, error) {
	c.mu.RLock()
	if time.Now().Before(c.outdated.expires) {
		items := cloneSlice(c.outdated.value)
		c.mu.RUnlock()
		c.cacheHits.Add(1)
		return items, nil
	}
	c.mu.RUnlock()
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("outdated", func() (interface{}, error) {
		items, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		c.mu.Lock()
		c.outdated = cacheSlice[brew.OutdatedPackage]{
			value:   cloneSlice(items),
			expires: time.Now().Add(outdatedTTL),
		}
		c.mu.Unlock()
		return cloneSlice(items), nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]brew.OutdatedPackage), nil
}

func (c *Cache) loadSearch(query string, loader func(string) ([]brew.SearchItem, error)) ([]brew.SearchItem, error) {
	key := strings.TrimSpace(strings.ToLower(query))
	if key == "" {
		key = query
	}

	c.mu.RLock()
	if cached, ok := c.searchByQuery[key]; ok && time.Now().Before(cached.expires) {
		items := cloneSlice(cached.value)
		c.mu.RUnlock()
		c.cacheHits.Add(1)
		return items, nil
	}
	c.mu.RUnlock()
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("search:"+key, func() (interface{}, error) {
		items, loadErr := loader(query)
		if loadErr != nil {
			return nil, loadErr
		}
		c.mu.Lock()
		c.searchByQuery[key] = cacheSlice[brew.SearchItem]{
			value:   cloneSlice(items),
			expires: time.Now().Add(searchTTL),
		}
		c.mu.Unlock()
		return cloneSlice(items), nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]brew.SearchItem), nil
}

func (c *Cache) loadDeps(packages []string, loader func() ([]string, error)) ([]string, error) {
	key := strings.Join(packages, "\x00")

	c.mu.RLock()
	if cached, ok := c.depsByQuery[key]; ok && time.Now().Before(cached.expires) {
		items := cloneSlice(cached.value)
		c.mu.RUnlock()
		c.cacheHits.Add(1)
		return items, nil
	}
	c.mu.RUnlock()
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("deps:"+key, func() (interface{}, error) {
		items, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		c.mu.Lock()
		c.depsByQuery[key] = cacheSlice[string]{
			value:   cloneSlice(items),
			expires: time.Now().Add(depsTTL),
		}
		c.mu.Unlock()
		return cloneSlice(items), nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]string), nil
}

func (c *Cache) loadLeaves(loader func() ([]string, error)) ([]string, error) {
	c.mu.RLock()
	if time.Now().Before(c.leaves.expires) {
		items := cloneSlice(c.leaves.value)
		c.mu.RUnlock()
		c.cacheHits.Add(1)
		return items, nil
	}
	c.mu.RUnlock()
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("leaves", func() (interface{}, error) {
		items, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		c.mu.Lock()
		c.leaves = cacheSlice[string]{
			value:   cloneSlice(items),
			expires: time.Now().Add(leavesTTL),
		}
		c.mu.Unlock()
		return cloneSlice(items), nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]string), nil
}

func (c *Cache) loadTapInfo(repo string, installedOnly bool, loader func() (*brew.TapInfo, error)) (*brew.TapInfo, error) {
	key := repo
	if installedOnly {
		key += ":installed"
	}

	c.mu.RLock()
	if cached, ok := c.tapInfoByKey[key]; ok && time.Now().Before(cached.expires) && cached.value != nil {
		val := copyTapInfo(cached.value)
		c.mu.RUnlock()
		c.cacheHits.Add(1)
		return val, nil
	}
	c.mu.RUnlock()
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("tap:"+key, func() (interface{}, error) {
		info, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		c.mu.Lock()
		c.tapInfoByKey[key] = cachePtr[brew.TapInfo]{
			value:   copyTapInfo(info),
			expires: time.Now().Add(tapInfoTTL),
		}
		c.mu.Unlock()
		return copyTapInfo(info), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*brew.TapInfo), nil
}

func (c *Cache) loadServices(scope string, loader func() ([]services.Service, error)) ([]services.Service, error) {
	key := strings.ToLower(strings.TrimSpace(scope))
	if key == "" {
		key = "default"
	}

	c.mu.RLock()
	if cached, ok := c.servicesByKey[key]; ok && time.Now().Before(cached.expires) {
		items := cloneSlice(cached.value)
		c.mu.RUnlock()
		c.cacheHits.Add(1)
		return items, nil
	}
	c.mu.RUnlock()
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("services:"+key, func() (interface{}, error) {
		items, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		c.mu.Lock()
		c.servicesByKey[key] = cacheSlice[services.Service]{
			value:   cloneSlice(items),
			expires: time.Now().Add(servicesListTTL),
		}
		c.mu.Unlock()
		return cloneSlice(items), nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]services.Service), nil
}

func (c *Cache) loadFormula(name string, loader func(string) (*brew.RemoteFormula, error)) (*brew.RemoteFormula, error) {
	if formula, ok := c.formulaMeta.Get(name); ok {
		c.cacheHits.Add(1)
		return formula, nil
	}
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("formula:"+name, func() (interface{}, error) {
		formula, loadErr := loader(name)
		if loadErr != nil {
			return nil, loadErr
		}
		c.formulaMeta.Put(name, formula)
		return formula, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*brew.RemoteFormula), nil
}

func (c *Cache) loadCask(name string, loader func(string) (*brew.CaskMetadata, error)) (*brew.CaskMetadata, error) {
	if metadata, ok := c.caskMeta.Get(name); ok {
		c.cacheHits.Add(1)
		return metadata, nil
	}
	c.cacheMisses.Add(1)

	v, err, _ := c.sf.Do("cask:"+name, func() (interface{}, error) {
		metadata, loadErr := loader(name)
		if loadErr != nil {
			return nil, loadErr
		}
		c.caskMeta.Put(name, metadata)
		return metadata, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*brew.CaskMetadata), nil
}

func (c *Cache) markWarmup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastWarmup = time.Now()
}

func (c *Cache) invalidate(event string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event {
	case EventInstalledChanged:
		c.installed = cacheSlice[brew.PackageInfo]{}
		c.outdated = cacheSlice[brew.OutdatedPackage]{}
		c.leaves = cacheSlice[string]{}
		c.depsByQuery = make(map[string]cacheSlice[string])
		c.searchByQuery = make(map[string]cacheSlice[brew.SearchItem])
	case EventTapChanged:
		c.tapInfoByKey = make(map[string]cachePtr[brew.TapInfo])
		c.searchByQuery = make(map[string]cacheSlice[brew.SearchItem])
	case EventIndexRefreshed:
		c.outdated = cacheSlice[brew.OutdatedPackage]{}
		c.searchByQuery = make(map[string]cacheSlice[brew.SearchItem])
		c.depsByQuery = make(map[string]cacheSlice[string])
		c.formulaMeta.Clear()
		c.caskMeta.Clear()
	case EventServiceChanged:
		c.servicesByKey = make(map[string]cacheSlice[services.Service])
	default:
		c.installed = cacheSlice[brew.PackageInfo]{}
		c.outdated = cacheSlice[brew.OutdatedPackage]{}
		c.leaves = cacheSlice[string]{}
		c.searchByQuery = make(map[string]cacheSlice[brew.SearchItem])
		c.depsByQuery = make(map[string]cacheSlice[string])
		c.tapInfoByKey = make(map[string]cachePtr[brew.TapInfo])
		c.servicesByKey = make(map[string]cacheSlice[services.Service])
		c.formulaMeta.Clear()
		c.caskMeta.Clear()
	}
}

func (c *Cache) stats(startedAt time.Time) StatsResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	uptime := time.Since(startedAt)
	var lastWarmup *time.Time
	if !c.lastWarmup.IsZero() {
		t := c.lastWarmup
		lastWarmup = &t
	}

	return StatsResponse{
		UptimeSeconds:      int64(uptime.Seconds()),
		RequestsTotal:      c.requestsTotal.Load(),
		CacheHits:          c.cacheHits.Load(),
		CacheMisses:        c.cacheMisses.Load(),
		LastWarmupAt:       lastWarmup,
		InstalledCached:    time.Now().Before(c.installed.expires),
		OutdatedCached:     time.Now().Before(c.outdated.expires),
		LeavesCached:       time.Now().Before(c.leaves.expires),
		FormulaMetaEntries: c.formulaMeta.Len(),
		CaskMetaEntries:    c.caskMeta.Len(),
		DepsCacheEntries:   len(c.depsByQuery),
		TapCacheEntries:    len(c.tapInfoByKey),
		ServicesEntries:    len(c.servicesByKey),
		SearchEntries:      len(c.searchByQuery),
	}
}

func cloneSlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func copyTapInfo(in *brew.TapInfo) *brew.TapInfo {
	if in == nil {
		return nil
	}
	out := *in
	out.Formulae = cloneSlice(in.Formulae)
	out.Casks = cloneSlice(in.Casks)
	out.Installed = cloneSlice(in.Installed)
	return &out
}
