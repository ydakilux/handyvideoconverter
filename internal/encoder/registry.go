package encoder

import "sync"

// Registry holds registered encoders keyed by codec name.
type Registry struct {
	mu       sync.RWMutex
	encoders map[string]Encoder
	order    []string
}

// NewRegistry creates an empty encoder registry.
func NewRegistry() *Registry {
	return &Registry{
		encoders: make(map[string]Encoder),
	}
}

// Register adds an encoder to the registry, keyed by its Name().
// If an encoder with the same name already exists it is replaced.
func (r *Registry) Register(enc Encoder) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := enc.Name()
	if _, exists := r.encoders[name]; !exists {
		r.order = append(r.order, name)
	}
	r.encoders[name] = enc
}

// Get retrieves an encoder by codec name.
func (r *Registry) Get(name string) (Encoder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	enc, ok := r.encoders[name]
	return enc, ok
}

// All returns all registered encoders in registration order.
func (r *Registry) All() []Encoder {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Encoder, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.encoders[name])
	}
	return result
}
