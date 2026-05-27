package schemapb

import (
	"context"
	"errors"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// Config holds SchemaService server settings. The zero value is fully
// locked down (no inline schemas, no registration); use DefaultConfig for a
// permissive setup and tighten from there.
type Config struct {
	// Registry backs RegisterSchema/GetSchema/ListSchemas and id-based refs.
	// Defaults to an InMemoryRegistry when nil.
	Registry Registry
	// AllowRegister permits the RegisterSchema RPC (clients adding schemas).
	// Disable to serve only a curated, pre-loaded set.
	AllowRegister bool
	// AllowInlineSchema permits Validate/Compute against a schema supplied
	// inline in the request (SchemaRef.schema), rather than only registered
	// schemas addressed by identity.
	AllowInlineSchema bool
	// CacheValidators compiles a *Validator once per registered schema and
	// reuses it. Inline schemas are never cached.
	CacheValidators bool
}

// DefaultConfig returns a permissive configuration backed by a fresh
// in-memory registry: registration, inline schemas and validator caching all
// enabled.
func DefaultConfig() Config {
	return Config{
		Registry:          NewInMemoryRegistry(),
		AllowRegister:     true,
		AllowInlineSchema: true,
		CacheValidators:   true,
	}
}

// Server implements SchemaServiceServer over a Registry and the validator /
// computed-value engine. It is safe for concurrent use.
type Server struct {
	UnimplementedSchemaServiceServer
	cfg   Config
	mu    sync.Mutex
	cache map[string]*Validator
}

// NewServer builds a Server from cfg. A nil Registry is replaced with a fresh
// InMemoryRegistry.
func NewServer(cfg Config) *Server {
	if cfg.Registry == nil {
		cfg.Registry = NewInMemoryRegistry()
	}
	return &Server{cfg: cfg, cache: map[string]*Validator{}}
}

// RegisterSchema validates and stores a schema.
func (s *Server) RegisterSchema(ctx context.Context, sc *Schema) (*RegisterSchemaResponse, error) {
	if !s.cfg.AllowRegister {
		return nil, status.Error(codes.PermissionDenied, "schema registration is disabled")
	}
	if errs := ValidateSchema(sc); len(errs) > 0 {
		return &RegisterSchemaResponse{Valid: false, Errors: errs}, nil
	}
	if err := s.cfg.Registry.Put(ctx, sc); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.invalidate(sc.GetId())
	return &RegisterSchemaResponse{Id: sc.GetId(), Valid: true}, nil
}

// GetSchema returns a registered schema.
func (s *Server) GetSchema(ctx context.Context, id *SchemaIdentity) (*Schema, error) {
	sc, err := s.cfg.Registry.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "schema %q not found", identityString(id))
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return sc, nil
}

// ListSchemas returns matching schema summaries.
func (s *Server) ListSchemas(ctx context.Context, f *Filter) (*ListSchemasResponse, error) {
	list, err := s.cfg.Registry.List(ctx, f)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	out := make([]*SchemaSummary, 0, len(list))
	for _, sc := range list {
		sum := &SchemaSummary{Id: sc.GetId()}
		if d := sc.GetDescription(); d != "" {
			sum.Description = Ptr(d)
		}
		out = append(out, sum)
	}
	return &ListSchemasResponse{Schemas: out}, nil
}

// ValidateSchema checks that a descriptor is well-formed.
func (s *Server) ValidateSchema(_ context.Context, sc *Schema) (*ValidateSchemaResponse, error) {
	errs := ValidateSchema(sc)
	return &ValidateSchemaResponse{Valid: len(errs) == 0, Errors: errs}, nil
}

// Validate validates form values against the referenced schema.
func (s *Server) Validate(ctx context.Context, req *ValidateRequest) (*ValidateResponse, error) {
	v, err := s.resolveValidator(ctx, req.GetSchema())
	if err != nil {
		return nil, err
	}
	errs := v.ValidateStruct(req.GetValues())
	return &ValidateResponse{Valid: !hasBlockingError(errs), Errors: errs}, nil
}

// Compute evaluates the schema's Computed fields for the given values.
func (s *Server) Compute(ctx context.Context, req *ComputeRequest) (*ComputeResponse, error) {
	v, err := s.resolveValidator(ctx, req.GetSchema())
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	if req.GetValues() != nil {
		values = req.GetValues().AsMap()
	}
	resolved, errs := v.Compute(values)
	st, perr := structpb.NewStruct(resolved)
	if perr != nil {
		return nil, status.Error(codes.Internal, "marshal resolved values: "+perr.Error())
	}
	return &ComputeResponse{Values: st, Errors: errs}, nil
}

// resolveValidator resolves a SchemaRef and returns a compiled validator,
// mapping registry / schema errors to gRPC statuses.
func (s *Server) resolveValidator(ctx context.Context, ref *SchemaRef) (*Validator, error) {
	sc, err := s.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	v, err := s.validatorFor(sc)
	if err != nil {
		var se *SchemaError
		if errors.As(err, &se) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid schema: %s", se.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return v, nil
}

func (s *Server) resolve(ctx context.Context, ref *SchemaRef) (*Schema, error) {
	if ref == nil {
		return nil, status.Error(codes.InvalidArgument, "schema ref is required")
	}
	switch {
	case ref.GetId() != nil:
		sc, err := s.cfg.Registry.Get(ctx, ref.GetId())
		if errors.Is(err, ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "schema %q not found", identityString(ref.GetId()))
		}
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return sc, nil
	case ref.GetSchema() != nil:
		if !s.cfg.AllowInlineSchema {
			return nil, status.Error(codes.PermissionDenied, "inline schemas are disabled")
		}
		return ref.GetSchema(), nil
	default:
		return nil, status.Error(codes.InvalidArgument, "schema ref must set id or schema")
	}
}

func (s *Server) validatorFor(sc *Schema) (*Validator, error) {
	var key string
	if s.cfg.CacheValidators && sc.GetId().GetName() != "" {
		key = identityKey(sc.GetId())
		s.mu.Lock()
		v, ok := s.cache[key]
		s.mu.Unlock()
		if ok {
			return v, nil
		}
	}
	v, err := NewValidator(sc)
	if err != nil {
		return nil, err
	}
	if key != "" {
		s.mu.Lock()
		s.cache[key] = v
		s.mu.Unlock()
	}
	return v, nil
}

func (s *Server) invalidate(id *SchemaIdentity) {
	s.mu.Lock()
	delete(s.cache, identityKey(id))
	s.mu.Unlock()
}

// hasBlockingError reports whether any error has ERROR (or unspecified)
// severity — i.e. a failure that blocks submit. WARNING does not block.
func hasBlockingError(errs []*FieldError) bool {
	for _, e := range errs {
		if e.GetSeverity() != Schema_Filed_WARNING {
			return true
		}
	}
	return false
}
