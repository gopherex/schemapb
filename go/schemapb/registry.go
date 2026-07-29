package schemapb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Registry errors. Implementations should return ErrNotFound from Get when a
// schema is absent.
var (
	ErrNotFound        = errors.New("schemapb: schema not found")
	ErrInvalidIdentity = errors.New("schemapb: schema identity requires a name")
)

// ListFilter narrows Registry.List. Empty fields are unconstrained; set
// fields must all match. NameContains is a case-insensitive substring match
// on the name. A nil *ListFilter matches everything.
type ListFilter struct {
	Namespace    string
	Name         string
	Version      string
	NameContains string
}

// Match reports whether id satisfies the filter.
func (f *ListFilter) Match(id *SchemaIdentity) bool {
	if f == nil {
		return true
	}
	if f.Namespace != "" && id.GetNamespace() != f.Namespace {
		return false
	}
	if f.Name != "" && id.GetName() != f.Name {
		return false
	}
	if f.Version != "" && id.GetVersion() != f.Version {
		return false
	}
	if f.NameContains != "" && !strings.Contains(strings.ToLower(id.GetName()), strings.ToLower(f.NameContains)) {
		return false
	}
	return true
}

// Registry stores schemas addressed by their identity. Implementations must
// be safe for concurrent use.
type Registry interface {
	// Put stores (or replaces) a schema under its own identity.
	Put(ctx context.Context, s *Schema) error
	// Get returns the schema for id, or ErrNotFound.
	Get(ctx context.Context, id *SchemaIdentity) (*Schema, error)
	// List returns the schemas matching filter (in unspecified order).
	List(ctx context.Context, filter *ListFilter) ([]*Schema, error)
}

// InMemoryRegistry is the default Registry: a concurrency-safe map. Nothing
// is persisted.
type InMemoryRegistry struct {
	mu sync.RWMutex
	m  map[string]*Schema
}

// NewInMemoryRegistry returns an empty in-memory registry.
func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{m: map[string]*Schema{}}
}

// Put stores (or replaces) a schema under its own identity.
func (r *InMemoryRegistry) Put(_ context.Context, s *Schema) error {
	if s.GetId().GetName() == "" {
		return ErrInvalidIdentity
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[identityKey(s.GetId())] = s
	return nil
}

// Get returns the schema for id, or ErrNotFound.
func (r *InMemoryRegistry) Get(_ context.Context, id *SchemaIdentity) (*Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.m[identityKey(id)]; ok {
		return s, nil
	}
	return nil, ErrNotFound
}

// List returns the schemas matching filter.
func (r *InMemoryRegistry) List(_ context.Context, f *ListFilter) ([]*Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Schema, 0, len(r.m))
	for _, s := range r.m {
		if f.Match(s.GetId()) {
			out = append(out, s)
		}
	}
	return out, nil
}

// =============================================================================
// Identity linking (reference composition)
// =============================================================================

// collectIDRefs walks fields recursively and records every identity-Ref by
// its identity key.
func collectIDRefs(fields []*Schema_Field, out map[string]*SchemaIdentity) {
	for _, f := range fields {
		if r := f.GetRef(); r != nil {
			if id := r.GetId(); id != nil {
				out[identityKey(id)] = id
			}
		}
		if l := f.GetList(); l != nil {
			collectIDRefs(l.GetItems(), out)
			continue
		}
		for _, child := range nestedSchemas(f) {
			collectIDRefs(child.GetFields(), out)
		}
	}
}

// Link resolves every identity-Ref in the schema against reg, pulling each
// referenced schema into the root defs (keyed by identity) so the schema
// becomes self-contained and validates standalone. The id-ref nodes keep
// their identity, so renderers can still see what each branch points at.
// Resolution is transitive: pulled-in schemas are themselves scanned for
// further id-refs. Link returns a linked clone; the receiver is not modified.
// An identity reg cannot supply, or a $defs key conflict, is an error.
func (s *Schema) Link(ctx context.Context, reg Registry) (*Schema, error) {
	root := proto.Clone(s).(*Schema)
	if root.Defs == nil {
		root.Defs = map[string]*Schema{}
	}
	if err := hoistDefs(root); err != nil {
		return nil, err
	}
	for {
		ids := map[string]*SchemaIdentity{}
		collectIDRefs(root.GetFields(), ids)
		for _, def := range root.Defs {
			collectIDRefs(def.GetFields(), ids)
		}
		added := false
		for key, id := range ids {
			if _, ok := root.Defs[key]; ok {
				continue
			}
			resolved, err := reg.Get(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("schemapb: link: cannot resolve schema %s: %w", identityString(id), err)
			}
			clone := proto.Clone(resolved).(*Schema)
			for k, d := range clone.GetDefs() {
				if err := addDef(root, k, d); err != nil {
					return nil, err
				}
			}
			clone.Defs = nil
			root.Defs[key] = clone
			added = true
		}
		if !added {
			break
		}
	}
	return root, nil
}
