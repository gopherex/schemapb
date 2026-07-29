package schemapb

import (
	"fmt"

	"golang.org/x/mod/semver"
)

// Distinct string domains of the public API get distinct Go types. Untyped
// string literals still convert implicitly (schemapb.Str("host") stays
// ergonomic), but a variable of one domain can no longer silently cross into
// another — a Namespace is not a SchemaName is not a FieldName.

// Namespace is a schema identity's grouping namespace.
type Namespace string

// SchemaName is a schema identity's name, unique within its namespace.
type SchemaName string

// FieldName names one field inside a schema.
type FieldName string

// DefName names a reusable sub-schema in the root defs map.
type DefName string

// TemplateName names a render template carried by a schema.
type TemplateName string

// RuleID is the stable id of an authored validation rule.
type RuleID string

// GroupName is a field's informative section label.
type GroupName string

// VariantKey selects a OneOf variant (the discriminator value).
type VariantKey string

// Format is a string-format registry identifier ("email", "k8s.quantity").
type Format string

// =============================================================================
// Version — an opaque, always-valid semver value
// =============================================================================

// Version is a validated semantic version. The field is unexported: a Version
// is either the zero value ("unversioned", serialises to "") or a canonical
// semver produced by Ver / ParseVersion / MustVersion — an invalid version is
// unrepresentable. Validation and ordering delegate to golang.org/x/mod/semver
// (the Go toolchain's own implementation).
type Version struct{ s string }

// Ver builds a release version ("v1.2.3").
func Ver(major, minor, patch uint64) Version {
	return Version{s: fmt.Sprintf("v%d.%d.%d", major, minor, patch)}
}

// ParseVersion parses a semver string ("v1.2.3", "v1.2.3-rc.1"; shorthands
// "v1" and "v1.2" canonicalise). An empty string is the zero Version. The
// leading "v" may be omitted on input; the canonical form carries it.
func ParseVersion(s string) (Version, error) {
	if s == "" {
		return Version{}, nil
	}
	if s[0] != 'v' {
		s = "v" + s
	}
	if !semver.IsValid(s) {
		return Version{}, fmt.Errorf("schemapb: invalid version %q", s)
	}
	return Version{s: semver.Canonical(s) + semver.Build(s)}, nil
}

// MustVersion is ParseVersion that panics on an invalid version.
func MustVersion(s string) Version {
	v, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

// IsZero reports the unversioned value.
func (v Version) IsZero() bool { return v.s == "" }

// String renders the canonical wire form ("" when unversioned).
func (v Version) String() string { return v.s }

// Compare orders versions by semver precedence (-1, 0, 1). The zero Version
// sorts before any real version.
func (v Version) Compare(o Version) int {
	switch {
	case v.s == o.s:
		return 0
	case v.s == "":
		return -1
	case o.s == "":
		return 1
	}
	return semver.Compare(v.s, o.s)
}

// =============================================================================
// Identity handle
// =============================================================================

// ID builds a schema identity. Declare it ONCE as a package-level value next
// to the schema and reuse the same variable everywhere the identity is needed
// (NewSchema, RefID, Registry.Get) — a typo then cannot compile:
//
//	var EndpointID = schemapb.ID("shared", "endpoint", schemapb.Ver(1, 0, 0))
func ID(ns Namespace, name SchemaName, ver Version) *SchemaIdentity {
	return &SchemaIdentity{Namespace: string(ns), Name: string(name), Version: ver.String()}
}

// Ns returns the namespace in its typed form.
func (id *SchemaIdentity) Ns() Namespace { return Namespace(id.GetNamespace()) }

// SchemaName returns the name in its typed form.
func (id *SchemaIdentity) SchemaName() SchemaName { return SchemaName(id.GetName()) }

// Ver returns the parsed version (zero when absent or unparseable).
func (id *SchemaIdentity) Ver() Version {
	v, _ := ParseVersion(id.GetVersion())
	return v
}
