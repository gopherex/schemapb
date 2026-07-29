package schemapb

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// Config holds SchemaService server settings. The zero value is fully locked
// down (no inline schemas, no registration); use DefaultConfig for a permissive
// setup and tighten from there. Compiled programs are cached globally by schema
// content hash, so there is no per-server validator cache to configure.
type Config struct {
	// Registry backs RegisterSchema/GetSchema/ListSchemas and id-based refs.
	// Defaults to an InMemoryRegistry when nil.
	Registry Registry
	// AllowRegister permits the RegisterSchema RPC (clients adding schemas).
	// Disable to serve only a curated, pre-loaded set.
	AllowRegister bool
	// AllowInlineSchema permits operations against a schema supplied inline in
	// the request (SchemaRef.schema), rather than only registered schemas.
	AllowInlineSchema bool
}

// DefaultConfig returns a permissive configuration backed by a fresh in-memory
// registry: registration and inline schemas both enabled.
func DefaultConfig() Config {
	return Config{
		Registry:          NewInMemoryRegistry(),
		AllowRegister:     true,
		AllowInlineSchema: true,
	}
}

// Server implements SchemaServiceServer over a Registry and the schema engine.
// It is safe for concurrent use.
type Server struct {
	UnimplementedSchemaServiceServer
	cfg Config
}

// NewServer builds a Server from cfg. A nil Registry is replaced with a fresh
// InMemoryRegistry.
func NewServer(cfg Config) *Server {
	if cfg.Registry == nil {
		cfg.Registry = NewInMemoryRegistry()
	}
	return &Server{cfg: cfg}
}

// RegisterSchema validates and stores a schema.
func (s *Server) RegisterSchema(ctx context.Context, sc *Schema) (*RegisterSchemaResponse, error) {
	if !s.cfg.AllowRegister {
		return nil, status.Error(codes.PermissionDenied, "schema registration is disabled")
	}
	if errs := sc.IsValid(); len(errs) > 0 {
		return &RegisterSchemaResponse{Valid: false, Errors: errs}, nil
	}
	if err := s.cfg.Registry.Put(ctx, sc); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
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
	errs := sc.IsValid()
	return &ValidateSchemaResponse{Valid: len(errs) == 0, Errors: errs}, nil
}

// Validate validates a Filled form against its schema.
func (s *Server) Validate(ctx context.Context, req *Filled) (*ValidateResponse, error) {
	sc, err := s.resolve(ctx, req.GetSchema())
	if err != nil {
		return nil, err
	}
	errs := sc.ValidateStruct(req.GetValues())
	return &ValidateResponse{Valid: !hasBlockingError(errs), Errors: errs}, nil
}

// Compute evaluates the Computed fields of a Filled form.
func (s *Server) Compute(ctx context.Context, req *Filled) (*ComputeResponse, error) {
	sc, err := s.resolve(ctx, req.GetSchema())
	if err != nil {
		return nil, err
	}
	resolved, errs := sc.ComputeStruct(req.GetValues())
	st, perr := structpb.NewStruct(resolved)
	if perr != nil {
		return nil, status.Error(codes.Internal, "marshal resolved values: "+perr.Error())
	}
	return &ComputeResponse{Values: st, Errors: errs}, nil
}

// Bake seals a Filled form into a Baked (validate + resolve).
func (s *Server) Bake(ctx context.Context, req *Filled) (*BakeResponse, error) {
	sc, err := s.resolve(ctx, req.GetSchema())
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	if req.GetValues() != nil {
		values = req.GetValues().AsMap()
	}
	baked, errs := sc.Bake(values)
	return &BakeResponse{Baked: baked, Errors: errs}, nil
}

// Merge layers overrides onto a Baked and re-seals.
func (s *Server) Merge(_ context.Context, req *MergeRequest) (*BakeResponse, error) {
	base := req.GetBase()
	if base == nil || base.GetSchema() == nil {
		return nil, status.Error(codes.InvalidArgument, "merge base with a schema is required")
	}
	replace := req.GetLists() == ListMerge_LIST_MERGE_REPLACE
	baked, errs := base.Merge(req.GetOverrides(), replace)
	return &BakeResponse{Baked: baked, Errors: errs}, nil
}

// resolve turns a SchemaRef into a schema: by identity from the registry, or
// inline when permitted.
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

// hasBlockingError reports whether any error has ERROR (or unspecified)
// severity — a failure that blocks submit. WARNING does not block.
func hasBlockingError(errs []*FieldError) bool {
	for _, e := range errs {
		if e.GetSeverity() != Schema_Filed_WARNING {
			return true
		}
	}
	return false
}
