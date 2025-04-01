package persistence

import (
	"container/list"
	"sync"
)

// LRUCache implements a thread-safe LRU cache for efficient memory usage
type LRUCache struct {
	capacity int
	items    map[string]*list.Element
	list     *list.List
	mu       sync.RWMutex
}

// cacheEntry represents an entry in the LRU cache
type cacheEntry struct {
	key   string
	value []byte
}

// NewLRUCache creates a new LRU cache with the specified capacity
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		list:     list.New(),
	}
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	element, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Move to front (most recently used)
	c.list.MoveToFront(element)
	return element.Value.(*cacheEntry).value, true
}

// Put adds or updates a value in the cache
func (c *LRUCache) Put(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if element, exists := c.items[key]; exists {
		// Update value and move to front
		c.list.MoveToFront(element)
		element.Value.(*cacheEntry).value = value
		return
	}

	// Add new entry
	element := c.list.PushFront(&cacheEntry{key: key, value: value})
	c.items[key] = element

	// Evict if over capacity
	if c.list.Len() > c.capacity {
		c.evict()
	}
}

// Remove removes a key from the cache
func (c *LRUCache) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, exists := c.items[key]; exists {
		c.list.Remove(element)
		delete(c.items, key)
	}
}

// evict removes the least recently used item
func (c *LRUCache) evict() {
	element := c.list.Back()
	if element != nil {
		entry := c.list.Remove(element).(*cacheEntry)
		delete(c.items, entry.key)
	}
}

// Len returns the number of items in the cache
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.list.Len()
}

// Clear removes all items from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.list = list.New()
}

// Keys returns all keys in the cache
func (c *LRUCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, c.list.Len())
	for element := c.list.Front(); element != nil; element = element.Next() {
		keys = append(keys, element.Value.(*cacheEntry).key)
	}

	return keys
}
