package plugin

import (
	"fmt"
	"sync"
)

// Registry manages the set of available plugin implementations.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]any
}

var (
	defaultRegistry = NewRegistry()
)

// DefaultRegistry returns the singleton plugin registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// NewRegistry creates a new instance of plugin Registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]any),
	}
}

// Register registers a plugin instance by its identifier.
func (r *Registry) Register(name string, instance any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[name] = instance
}

// Get retrieves a registered plugin instance by name.
func (r *Registry) Get(name string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// RegisterGlobal registers a plugin into the default singleton registry.
func RegisterGlobal(name string, instance any) {
	defaultRegistry.Register(name, instance)
}

// GetGlobal gets a plugin from the default singleton registry.
func GetGlobal(name string) (any, error) {
	p, ok := defaultRegistry.Get(name)
	if !ok {
		return nil, fmt.Errorf("plugin %q not found in registry", name)
	}
	return p, nil
}
