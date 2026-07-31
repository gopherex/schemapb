// Package schemapb is the Go reference implementation of the schemapb
// contract: a runtime, proto-defined form/config schema descriptor with a
// validation and derived-value engine.
//
// A producer builds a Schema (fluent builders, see NewSchema) and ships it as
// a protobuf message; any consumer — this package, or the TypeScript, Python
// and Rust implementations — validates values against it, resolves defaults
// and Computed fields (CEL expressions), and renders Mustache templates.
// Cross-implementation agreement is pinned by the conformance suite
// (conformance/golden), not by sharing a runtime.
//
// Core entry points:
//
//	Compile(schema, opts...)   — compile once into an *Engine (explicit form)
//	(*Schema).Validate/Resolve/Bake/Render — sugar over a cached engine
//	ID / Ver                   — typed identity handles (declare once, reuse)
//	FromGo / ToGo / CanonicalValue — the typed Value boundary
//	Registry / Link            — identity-based schema composition
package schemapb
