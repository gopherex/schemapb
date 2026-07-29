package schemapb

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// Registry errors. Implementations should return ErrNotFound from Get when a
// schema is absent so the server can map it to a NotFound status.
var (
	ErrNotFound        = errors.New("schema not found")
	ErrInvalidIdentity = errors.New("schema identity requires a name")
)

// Registry stores schemas addressed by their identity. Implementations must be
// safe for concurrent use.
type Registry interface {
	// Put stores (or replaces) a schema under its own identity.
	Put(ctx context.Context, s *Schema) error
	// Get returns the schema for id, or ErrNotFound.
	Get(ctx context.Context, id *SchemaIdentity) (*Schema, error)
	// List returns the schemas matching filter (in unspecified order). A nil
	// filter, or one with empty fields, matches everything.
	List(ctx context.Context, filter *Filter) ([]*Schema, error)
}

// identityKey is the registry key for an identity: namespace, name and version
// are independent coordinates, so all three form the key.
func identityKey(id *SchemaIdentity) string {
	return id.GetNamespace() + "\x00" + id.GetName() + "\x00" + id.GetVersion()
}

// identityString renders an identity for human-readable messages.
func identityString(id *SchemaIdentity) string {
	s := id.GetName()
	if ns := id.GetNamespace(); ns != "" {
		s = ns + "/" + s
	}
	if v := id.GetVersion(); v != "" {
		s += "@" + v
	}
	return s
}

// matchFilter reports whether id satisfies f (nil f matches everything). Empty
// fields are not constrained; set fields must all match (AND). NameContains is
// a case-insensitive substring match on the name.
func matchFilter(id *SchemaIdentity, f *Filter) bool {
	if f.GetNamespace() != "" && id.GetNamespace() != f.GetNamespace() {
		return false
	}
	if f.GetName() != "" && id.GetName() != f.GetName() {
		return false
	}
	if f.GetVersion() != "" && id.GetVersion() != f.GetVersion() {
		return false
	}
	if c := f.GetNameContains(); c != "" && !strings.Contains(strings.ToLower(id.GetName()), strings.ToLower(c)) {
		return false
	}
	return true
}

// InMemoryRegistry is the default Registry: a concurrency-safe map. Suitable
// for the expected handful of schemas; nothing is persisted.
type InMemoryRegistry struct {
	mu sync.RWMutex
	m  map[string]*Schema
}

// NewInMemoryRegistry returns an empty in-memory registry.
func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{m: map[string]*Schema{}}
}

func (r *InMemoryRegistry) Put(_ context.Context, s *Schema) error {
	if s.GetId().GetName() == "" {
		return ErrInvalidIdentity
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[identityKey(s.GetId())] = s
	return nil
}

func (r *InMemoryRegistry) Get(_ context.Context, id *SchemaIdentity) (*Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.m[identityKey(id)]; ok {
		return s, nil
	}
	return nil, ErrNotFound
}

func (r *InMemoryRegistry) List(_ context.Context, f *Filter) ([]*Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Schema, 0, len(r.m))
	for _, s := range r.m {
		if matchFilter(s.GetId(), f) {
			out = append(out, s)
		}
	}
	return out, nil
}
