package sources

// Registry maps source IDs to constructed Source impls. Populated by main.go
// during plugin startup based on per-source config.
type Registry struct {
	sources map[string]Source
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{sources: map[string]Source{}}
}

// Register adds a source. Last registration wins for a given ID.
func (r *Registry) Register(s Source) {
	r.sources[s.ID()] = s
}

// ForID returns the source for ID, or nil if not registered.
func (r *Registry) ForID(id string) Source {
	return r.sources[id]
}

// All returns the registered sources in unspecified order.
func (r *Registry) All() []Source {
	out := make([]Source, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, s)
	}
	return out
}
