package drive

import (
	"sync"
	"time"
)

type fileCache struct {
	nodes sync.Map
}

func newFileCache(root Entry) *fileCache {
	cache := new(fileCache)
	cache.newNode(root)
	return cache
}

func (c *fileCache) load(id string) *node {
	if val, ok := c.nodes.Load(id); ok {
		return val.(*node)
	}
	return nil
}

func (c *fileCache) alias(id string, node *node) {
	if id != "" && node != nil {
		c.nodes.Store(id, node)
	}
}

func (c *fileCache) newNode(file Entry) *node {
	node := &node{info: file, cache: c}
	c.nodes.Store(file.ID(), node)
	return node
}

type node struct {
	info   Entry
	cache  *fileCache
	node   sync.Map
	mu     sync.RWMutex
	exp    time.Time
	loaded bool
}

func (n *node) invalid() { n.mu.Lock(); n.loaded = false; n.mu.Unlock() }
func (n *node) valid() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.loaded && n.exp.After(time.Now())
}
func (n *node) enable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.exp = time.Now().Add(time.Minute)
	n.loaded = true
}
func (n *node) add(children ...Entry) {
	for _, child := range children {
		node := n.cache.newNode(child)
		n.node.Store(child.Name(), node)
	}
}
func (n *node) search(name string, loader func() (Entry, error)) (Entry, error) {
	if val, ok := n.node.Load(name); ok {
		return val.(*node).info, nil
	}
	result, err := loader()
	if err != nil {
		return nil, err
	}
	n.add(result)
	return result, nil
}

func (n *node) list(loader func() ([]Entry, error)) ([]Entry, error) {
	if n.valid() {
		result := make([]Entry, 0)
		n.node.Range(func(key, value any) bool {
			result = append(result, value.(*node).info)
			return true
		})
		return result, nil
	}
	result, err := loader()
	if err != nil {
		return nil, err
	}
	n.node.Range(func(key, value any) bool {
		n.node.Delete(key)
		return true
	})
	n.add(result...)
	n.enable()
	return result, nil
}

func (n *node) delete(child Entry) {
	p := n.cache.load(child.ID())
	if child.IsDir() && p != nil {
		p.node.Range(func(key, value any) bool {
			p.delete(value.(*node).info)
			return true
		})
	}
	n.cache.nodes.Delete(child.ID())
	n.node.Delete(child.Name())
	n.invalid()
}

func (c *fileCache) invalid(files ...Entry) {
	for _, file := range files {
		if file == nil {
			continue
		}
		node := c.load(file.ParentID())
		if node != nil {
			node.delete(file)
		}
	}
}
