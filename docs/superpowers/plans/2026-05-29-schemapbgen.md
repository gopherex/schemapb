# schemapbgen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A CLI (`cmd/schemapbgen`) that reads a `schemapb.Schema` (from protojson, later from Go builder code) and emits a typed Go struct mirror with full roundtrip/validation/sugar, losing no dynamic rule.

**Architecture:** Three pure stages + a CLI shell. `parse` loads a `*schemapb.Schema`. `model` walks it into a naming-resolved IR (`File` → `Type`s → `Field`s, enums, oneofs, defs) with protobuf-style `_`-nested names and a hard collision check. `emit` renders the IR to gofmt'd Go source, embedding the original schema as protobuf wire bytes for runtime validation. The CLI lives in its own Go module so cobra deps stay out of the library module.

**Tech Stack:** Go 1.25+, `google.golang.org/protobuf` (proto + protojson + structpb), `github.com/stroppy-io/schemapb/schemapb` (engine, types), cobra (CLI only), `go/format` (output), golden-file tests.

**Spec:** `docs/superpowers/specs/2026-05-29-schemapbgen-design.md`

---

## File structure

```
cmd/schemapbgen/
  go.mod                      # own module: github.com/stroppy-io/schemapb/cmd/schemapbgen
  go.sum
  main.go                     # cobra root, wires flags -> parse -> model -> emit -> write
  internal/
    parse/
      parse.go                # protojson -> *schemapb.Schema (Phase 1)
      bridge.go               # -from-go-code: temp main + go run -> schema dump (Phase 2)
    model/
      ir.go                   # IR types: File, Type, Field, EnumDef, OneOfDef, Kind
      names.go                # SchemaIdentity/field -> Go identifier; PascalCase, version sanitize
      build.go                # walk *schemapb.Schema -> *File (collision check here)
      build_test.go
      names_test.go
    emit/
      emit.go                 # File -> gofmt'd []byte (orchestrates renderers below)
      structs.go              # struct decls, fields, json tags, oneof iface, enum type+consts
      sugar.go                # getters, builder, Clone, Marshal/UnmarshalJSON
      roundtrip.go            # ToValues / FromValues / ToFilled / ToBaked
      schemawrap.go           # _schema wire bytes, Schema(), Validate(), Identity, Default()
      emit_test.go
      testdata/               # golden: *.json input -> *.go.golden expected output
```

Generated code imports `github.com/stroppy-io/schemapb/schemapb`, `google.golang.org/protobuf/proto`, `.../types/known/structpb`, `time`, `sync`.

---

## Task 1: Bootstrap the CLI module

**Files:**
- Create: `cmd/schemapbgen/go.mod`
- Create: `cmd/schemapbgen/main.go`

- [ ] **Step 1: Create the module file**

`cmd/schemapbgen/go.mod`:

```
module github.com/stroppy-io/schemapb/cmd/schemapbgen

go 1.25.0

require (
	github.com/spf13/cobra v1.8.1
	github.com/stroppy-io/schemapb v0.0.0
	google.golang.org/protobuf v1.36.11
)

replace github.com/stroppy-io/schemapb => ../..
```

(The `replace` points the CLI module at the local library during development; a tagged version replaces it for release.)

- [ ] **Step 2: Minimal main that compiles**

`cmd/schemapbgen/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schemapbgen",
		Short: "Generate typed Go structs from schemapb schemas",
	}
}
```

- [ ] **Step 3: Resolve deps and build**

Run: `cd cmd/schemapbgen && go mod tidy && go build ./...`
Expected: builds, `go.sum` written.

- [ ] **Step 4: Commit**

```bash
git add cmd/schemapbgen/go.mod cmd/schemapbgen/go.sum cmd/schemapbgen/main.go
git commit -m "feat(schemapbgen): bootstrap CLI module"
```

---

## Task 2: Naming — identity & field → Go identifiers

**Files:**
- Create: `cmd/schemapbgen/internal/model/names.go`
- Test: `cmd/schemapbgen/internal/model/names_test.go`

- [ ] **Step 1: Write the failing test**

`names_test.go`:

```go
package model

import (
	"testing"

	"github.com/stroppy-io/schemapb/schemapb"
)

func TestRootName(t *testing.T) {
	cases := []struct {
		ns, name, ver string
		want          string
	}{
		{"infra", "disk", "v1", "InfraDiskV1"},
		{"", "user", "v2", "UserV2"},
		{"infra", "disk", "", "InfraDisk"},
		{"infra", "disk_config", "1.2.0", "InfraDiskConfigV1_2_0"},
		{"", "user-profile", "", "UserProfile"},
	}
	for _, c := range cases {
		id := &schemapb.SchemaIdentity{Namespace: c.ns, Name: c.name, Version: c.ver}
		if got := RootName(id); got != c.want {
			t.Errorf("RootName(%q,%q,%q)=%q want %q", c.ns, c.name, c.ver, got, c.want)
		}
	}
}

func TestPascal(t *testing.T) {
	cases := map[string]string{
		"shared_buffers": "SharedBuffers",
		"wal-level":      "WalLevel",
		"s3":             "S3",
		"v1":             "V1",
	}
	for in, want := range cases {
		if got := pascal(in); got != want {
			t.Errorf("pascal(%q)=%q want %q", in, got, want)
		}
	}
}

func TestChild(t *testing.T) {
	if got := Child("InfraDiskV1", "wal"); got != "InfraDiskV1_Wal" {
		t.Errorf("Child=%q", got)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/model/ -run TestRootName -v`
Expected: FAIL (undefined: RootName).

- [ ] **Step 3: Implement**

`names.go`:

```go
// Package model turns a *schemapb.Schema into a naming-resolved IR.
package model

import (
	"strings"
	"unicode"

	"github.com/stroppy-io/schemapb/schemapb"
)

// RootName derives the root Go type name from a schema identity:
// namespace+name+version, each segment PascalCased, empty parts skipped.
func RootName(id *schemapb.SchemaIdentity) string {
	var b strings.Builder
	if id.GetNamespace() != "" {
		b.WriteString(pascal(id.GetNamespace()))
	}
	b.WriteString(pascal(id.GetName()))
	if v := id.GetVersion(); v != "" {
		b.WriteString(versionSeg(v))
	}
	return b.String()
}

// Child appends a nesting segment protobuf-style: Parent_Child.
func Child(parent, name string) string { return parent + "_" + pascal(name) }

// pascal splits on _ - . and uppercases each word's first rune.
func pascal(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		rs := []rune(p)
		b.WriteRune(unicode.ToUpper(rs[0]))
		b.WriteString(string(rs[1:]))
	}
	return b.String()
}

// versionSeg turns a version into a valid identifier segment: dots become _,
// each dot-separated part is PascalCased, e.g. "1.2.0" -> "V1_2_0", "v1" -> "V1".
func versionSeg(v string) string {
	parts := strings.Split(v, ".")
	for i, p := range parts {
		parts[i] = pascal(p)
	}
	seg := strings.Join(parts, "_")
	if seg == "" {
		return ""
	}
	// Ensure leading "V" if the version starts with a digit (e.g. "1.2.0").
	if r := []rune(seg)[0]; unicode.IsDigit(r) {
		seg = "V" + seg
	}
	return seg
}
```

- [ ] **Step 4: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./internal/model/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schemapbgen/internal/model/names.go cmd/schemapbgen/internal/model/names_test.go
git commit -m "feat(schemapbgen): identity/field name resolution"
```

---

## Task 3: The IR types

**Files:**
- Create: `cmd/schemapbgen/internal/model/ir.go`

- [ ] **Step 1: Define the IR (no test — pure data; exercised in Task 4)**

`ir.go`:

```go
package model

import "github.com/stroppy-io/schemapb/schemapb"

// File is everything generated for one input schema.
type File struct {
	Package  string
	Root     string                  // root Go type name, e.g. InfraDiskV1
	Identity *schemapb.SchemaIdentity // for the Identity constant
	Wire     []byte                  // proto.Marshal of the original schema
	Types    []*Type                 // root + all nested structs, in declaration order
	Enums    []*EnumDef
}

// Type is one generated struct (root, nested object, oneof variant, or def).
type Type struct {
	Name   string   // full Go name, e.g. InfraDiskV1_Wal
	Doc    string   // schema description, if any
	Fields []*Field
	// OneOf, if non-nil, means this Type is the parent holding a oneof field;
	// variants are separate Types whose IfaceName == OneOf.IfaceName.
}

// Field is one struct field.
type Field struct {
	Name     string  // Go field name (PascalCase)
	JSONName string  // original schema field name
	GoType   string  // rendered Go type, e.g. "int64", "*InfraDiskV1_Wal", "[]string"
	Pointer  bool    // emitted as *T
	OmitEmpty bool
	Doc      string  // includes // when:/ // rule:/ // computed: lines
	Computed bool    // omitted on ToValues
	OneOf    *OneOfDef // non-nil if this field is a discriminated union
}

// OneOfDef describes a oneof field.
type OneOfDef struct {
	IfaceName     string            // e.g. InfraDiskV1_Storage
	Discriminator string            // value key selecting the variant
	Variants      map[string]string // discriminator value -> variant Go type name
}

// EnumDef is a generated enum type.
type EnumDef struct {
	Name   string           // e.g. InfraDiskV1_WalLevel
	Values map[int32]string // value -> label (from schema)
}

// Kind classifies a field's schema kind for type mapping.
type Kind int

const (
	KindScalar Kind = iota
	KindEnum
	KindDuration
	KindTimestamp
	KindList
	KindObject
	KindOneOf
	KindRef
	KindComputed
)
```

- [ ] **Step 2: Build**

Run: `cd cmd/schemapbgen && go build ./internal/model/`
Expected: builds.

- [ ] **Step 3: Commit**

```bash
git add cmd/schemapbgen/internal/model/ir.go
git commit -m "feat(schemapbgen): IR types"
```

---

## Task 4: Build IR from a schema — scalars, pointers, nesting

**Files:**
- Create: `cmd/schemapbgen/internal/model/build.go`
- Test: `cmd/schemapbgen/internal/model/build_test.go`

This task covers scalar kinds, pointer rules, nested Object recursion, and the
collision check. Enum/List/OneOf/Ref/Computed/Duration/Timestamp are added in
Task 5 (kept separate to stay bite-sized); `goScalar` already routes them to a
`"any"` placeholder so the walk compiles, and Task 5 replaces that.

- [ ] **Step 1: Write the failing test**

`build_test.go`:

```go
package model

import (
	"testing"

	"github.com/stroppy-io/schemapb/schemapb"
)

func TestBuildScalars(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
		schemapb.Str("wal_level"),
		schemapb.Object("wal", schemapb.Bool("enabled").Required()),
	).MustBuild()

	f, err := Build(s, "myconfig")
	if err != nil {
		t.Fatal(err)
	}
	if f.Root != "InfraDiskV1" {
		t.Fatalf("root=%q", f.Root)
	}
	root := findType(f, "InfraDiskV1")
	if root == nil {
		t.Fatal("no root type")
	}
	// required scalar -> value
	assertField(t, root, "SharedBuffers", "int64", false)
	// optional scalar -> pointer
	assertField(t, root, "WalLevel", "string", true)
	// object -> pointer to nested type
	assertField(t, root, "Wal", "*InfraDiskV1_Wal", true)
	// nested type exists
	if findType(f, "InfraDiskV1_Wal") == nil {
		t.Fatal("nested InfraDiskV1_Wal missing")
	}
}

func TestBuildCollisionFails(t *testing.T) {
	// Two object fields whose nested names would collide is impossible under
	// prefixing; a forced duplicate type name must error. Simulate by building
	// then asserting Build rejects a manually duplicated field name at same level.
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Object("wal", schemapb.Bool("a")),
	).MustBuild()
	// Inject a duplicate top-level field with the same name to force a clash.
	s.Fields = append(s.Fields, s.Fields[0])
	if _, err := Build(s, "p"); err == nil {
		t.Fatal("expected collision error")
	}
}

func findType(f *File, name string) *Type {
	for _, t := range f.Types {
		if t.Name == name {
			return t
		}
	}
	return nil
}

func assertField(t *testing.T, typ *Type, name, goType string, ptr bool) {
	t.Helper()
	for _, fld := range typ.Fields {
		if fld.Name == name {
			if fld.GoType != goType || fld.Pointer != ptr {
				t.Errorf("%s: goType=%q ptr=%v want %q %v", name, fld.GoType, fld.Pointer, goType, ptr)
			}
			return
		}
	}
	t.Errorf("field %s not found", name)
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/model/ -run TestBuild -v`
Expected: FAIL (undefined: Build).

- [ ] **Step 3: Implement**

`build.go`:

```go
package model

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/schemapb/schemapb"
)

// Build walks a schema into a File. It returns an error on a name collision
// (two generated types resolving to the same Go identifier).
func Build(s *schemapb.Schema, pkg string) (*File, error) {
	wire, err := proto.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	b := &builder{
		file:  &File{Package: pkg, Root: RootName(s.GetId()), Identity: s.GetId(), Wire: wire},
		seen:  map[string]bool{},
		defs:  s.GetDefs(),
	}
	b.walkSchema(s, b.file.Root, s.GetDescription())
	if b.err != nil {
		return nil, b.err
	}
	return b.file, nil
}

type builder struct {
	file *File
	seen map[string]bool // type names already declared
	defs map[string]*schemapb.Schema
	err  error
}

// walkSchema emits a Type named `name` for the schema's fields and recurses.
func (b *builder) walkSchema(s *schemapb.Schema, name, doc string) {
	if b.err != nil {
		return
	}
	if b.seen[name] {
		b.err = fmt.Errorf("type name collision: %q generated twice", name)
		return
	}
	b.seen[name] = true
	t := &Type{Name: name, Doc: doc}
	b.file.Types = append(b.file.Types, t)
	for _, f := range s.GetFields() {
		fld := b.field(f, name)
		if b.err != nil {
			return
		}
		t.Fields = append(t.Fields, fld)
	}
}

// field builds one Field and recurses into nested kinds.
func (b *builder) field(f *schemapb.Schema_Filed, parent string) *Field {
	fld := &Field{
		Name:     pascal(f.GetName()),
		JSONName: f.GetName(),
		Doc:      fieldDoc(f),
	}
	ptr := !f.GetRequired() || f.GetNullable()
	switch k := f.GetKind().(type) {
	case *schemapb.Schema_Filed_Object_:
		child := Child(parent, f.GetName())
		b.walkSchema(k.Object.GetSchema(), child, f.GetDescription())
		fld.GoType = "*" + child
		fld.Pointer = true
		fld.OmitEmpty = true
		return fld
	default:
		fld.GoType = goScalar(f)
		fld.Pointer = ptr
		if ptr {
			fld.GoType = "*" + fld.GoType
			fld.OmitEmpty = true
		}
	}
	return fld
}

// goScalar maps the simple scalar kinds. Non-scalar kinds return "any" for now;
// Task 5 replaces this with enum/list/oneof/ref/computed/duration/timestamp.
func goScalar(f *schemapb.Schema_Filed) string {
	switch f.GetKind().(type) {
	case *schemapb.Schema_Filed_Float_:
		return "float32"
	case *schemapb.Schema_Filed_Double_:
		return "float64"
	case *schemapb.Schema_Filed_Int32_:
		return "int32"
	case *schemapb.Schema_Filed_Int64_:
		return "int64"
	case *schemapb.Schema_Filed_Uint32:
		return "uint32"
	case *schemapb.Schema_Filed_Uint64:
		return "uint64"
	case *schemapb.Schema_Filed_Bool_:
		return "bool"
	case *schemapb.Schema_Filed_String_:
		return "string"
	default:
		return "any"
	}
}

// fieldDoc assembles the doc comment: description + dynamic-logic markers.
func fieldDoc(f *schemapb.Schema_Filed) string {
	var lines []string
	if d := f.GetDescription(); d != "" {
		lines = append(lines, d)
	}
	if w := f.GetWhen(); w != "" {
		lines = append(lines, "when: "+w)
	}
	if n := f.GetNormalize(); n != "" {
		lines = append(lines, "normalize: "+n)
	}
	for _, r := range f.GetRules() {
		lines = append(lines, "rule: "+r.GetExpr())
	}
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
```

- [ ] **Step 4: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./internal/model/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schemapbgen/internal/model/build.go cmd/schemapbgen/internal/model/build_test.go
git commit -m "feat(schemapbgen): build IR — scalars, pointers, nested objects, collision check"
```

---

## Task 5: Build IR for remaining kinds — enum, list, duration, timestamp, ref, computed, oneof

**Files:**
- Modify: `cmd/schemapbgen/internal/model/build.go`
- Test: `cmd/schemapbgen/internal/model/build_test.go`

- [ ] **Step 1: Add failing tests**

Append to `build_test.go`:

```go
func TestBuildKinds(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").
		Def("node", schemapb.Str("id").Required()).
		Fields(
			schemapb.Enum("level").Values(map[int32]string{0: "minimal", 1: "replica"}).Required(),
			schemapb.List("tags", schemapb.Str("v")).Required(),
			schemapb.Duration("ttl").Required(),
			schemapb.Timestamp("at").Required(),
			schemapb.Computed("eff", "root.shared * 2").Result(schemapb.ResultInt64),
			schemapb.Ref("parent", "node"),
			schemapb.OneOf("storage", "kind").
				Variant("local", schemapb.Str("path").Required()).
				Variant("s3", schemapb.Str("bucket").Required()),
		).MustBuild()

	f, err := Build(s, "p")
	if err != nil {
		t.Fatal(err)
	}
	root := findType(f, "InfraDiskV1")
	assertField(t, root, "Level", "InfraDiskV1_Level", false)        // enum named type
	assertField(t, root, "Tags", "[]string", false)                 // list of scalar
	assertField(t, root, "Ttl", "time.Duration", false)
	assertField(t, root, "At", "time.Time", false)
	assertField(t, root, "Parent", "*InfraDiskV1_Node", true)        // ref -> pointer to def
	assertField(t, root, "Storage", "InfraDiskV1_Storage", false)    // oneof iface

	if findType(f, "InfraDiskV1_Node") == nil {
		t.Error("def type InfraDiskV1_Node missing")
	}
	if findType(f, "InfraDiskV1_Storage_Local") == nil || findType(f, "InfraDiskV1_Storage_S3") == nil {
		t.Error("oneof variant types missing")
	}
	// enum registered
	var hasEnum bool
	for _, e := range f.Enums {
		if e.Name == "InfraDiskV1_Level" {
			hasEnum = true
		}
	}
	if !hasEnum {
		t.Error("enum InfraDiskV1_Level not registered")
	}
	// computed marked
	for _, fld := range root.Fields {
		if fld.Name == "Eff" && !fld.Computed {
			t.Error("Eff should be Computed")
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/model/ -run TestBuildKinds -v`
Expected: FAIL (Level maps to "any", etc.).

- [ ] **Step 3: Implement — replace the `field` switch and `goScalar` default**

In `build.go`, replace the `field` method's `switch` body with the full version:

```go
func (b *builder) field(f *schemapb.Schema_Filed, parent string) *Field {
	fld := &Field{Name: pascal(f.GetName()), JSONName: f.GetName(), Doc: fieldDoc(f)}
	ptr := !f.GetRequired() || f.GetNullable()
	switch k := f.GetKind().(type) {

	case *schemapb.Schema_Filed_Object_:
		child := Child(parent, f.GetName())
		b.walkSchema(k.Object.GetSchema(), child, f.GetDescription())
		fld.GoType, fld.Pointer, fld.OmitEmpty = "*"+child, true, true

	case *schemapb.Schema_Filed_Enum_:
		name := Child(parent, f.GetName())
		b.file.Enums = append(b.file.Enums, &EnumDef{Name: name, Values: k.Enum.GetValues()})
		fld.GoType = name
		b.applyPtr(fld, ptr)

	case *schemapb.Schema_Filed_Duration_:
		fld.GoType = "time.Duration"
		b.applyPtr(fld, ptr)

	case *schemapb.Schema_Filed_Timestamp_:
		fld.GoType = "time.Time"
		b.applyPtr(fld, ptr)

	case *schemapb.Schema_Filed_List_:
		fld.GoType = "[]" + b.listElem(k.List, parent, f.GetName())
		fld.OmitEmpty = true // slices already nil-able; no extra pointer

	case *schemapb.Schema_Filed_Ref_:
		fld.GoType = "*" + b.refType(k.Ref, parent)
		fld.Pointer, fld.OmitEmpty = true, true

	case *schemapb.Schema_Filed_Computed_:
		fld.Computed, fld.OmitEmpty = true, true
		fld.GoType = computedGoType(k.Computed.GetResult())
		if !f.GetRequired() {
			fld.GoType, fld.Pointer = "*"+fld.GoType, true
		}

	case *schemapb.Schema_Filed_OneOf_:
		iface := Child(parent, f.GetName())
		od := &OneOfDef{IfaceName: iface, Discriminator: k.OneOf.GetDiscriminator(), Variants: map[string]string{}}
		// stable order: sort variant keys
		for _, key := range sortedKeys(k.OneOf.GetVariants()) {
			vname := Child(iface, key)
			b.walkSchema(k.OneOf.GetVariants()[key], vname, "")
			od.Variants[key] = vname
		}
		fld.OneOf, fld.GoType = od, iface

	default:
		fld.GoType = goScalar(f)
		b.applyPtr(fld, ptr)
	}
	return fld
}

// applyPtr wraps a value type in a pointer when the field is optional/nullable.
func (b *builder) applyPtr(fld *Field, ptr bool) {
	if ptr {
		fld.GoType, fld.Pointer, fld.OmitEmpty = "*"+fld.GoType, true, true
	}
}

// listElem maps a list's element type. Single scalar item -> its scalar type;
// an object/multi-field item -> a generated <Parent>_<Field>Item struct.
func (b *builder) listElem(l *schemapb.Schema_Filed_List, parent, field string) string {
	items := l.GetItems()
	if len(items) == 1 {
		if s := goScalar(items[0]); s != "any" {
			return s
		}
	}
	itemName := Child(parent, field) + "Item" // e.g. InfraDiskV1_TagsItem (note: Child already adds _)
	// Child(parent,"tags") = parent_Tags; we want parent_TagsItem:
	itemName = Child(parent, field+"_x")       // placeholder; corrected below
	itemName = parent + "_" + pascal(field) + "Item"
	sub := &schemapb.Schema{Fields: items}
	b.walkSchema(sub, itemName, "")
	return itemName
}

// refType resolves a Ref to its generated def type name (local defs only here;
// id-refs are resolved to the def key by the engine's Link — Phase 1 supports
// name-target refs, which is what the builder emits).
func (b *builder) refType(r *schemapb.Schema_Filed_Ref, parent string) string {
	name := r.GetName()
	if name == "" {
		b.err = fmt.Errorf("id-target Ref not supported in Phase 1; use a named def")
		return "any"
	}
	defName := b.file.Root + "_" + pascal(name)
	if !b.seen[defName] {
		b.walkSchema(b.defs[name], defName, "")
	}
	return defName
}

func computedGoType(rt schemapb.Schema_Filed_ResultType) string {
	switch rt {
	case schemapb.ResultDouble:
		return "float64"
	case schemapb.ResultInt64:
		return "int64"
	case schemapb.ResultUint64:
		return "uint64"
	case schemapb.ResultBool:
		return "bool"
	case schemapb.ResultString:
		return "string"
	case schemapb.ResultDuration:
		return "time.Duration"
	default:
		return "any"
	}
}

func sortedKeys(m map[string]*schemapb.Schema) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
```

Add `"sort"` to the imports. Delete the now-dead placeholder lines in `listElem`
(keep only the final `itemName = parent + "_" + pascal(field) + "Item"`).

- [ ] **Step 4: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./internal/model/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schemapbgen/internal/model/build.go cmd/schemapbgen/internal/model/build_test.go
git commit -m "feat(schemapbgen): build IR for enum/list/duration/timestamp/ref/computed/oneof"
```

---

## Task 6: Emit — structs, enums, oneof interfaces

**Files:**
- Create: `cmd/schemapbgen/internal/emit/emit.go`
- Create: `cmd/schemapbgen/internal/emit/structs.go`
- Test: `cmd/schemapbgen/internal/emit/emit_test.go`

- [ ] **Step 1: Write the failing test**

`emit_test.go`:

```go
package emit

import (
	"strings"
	"testing"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/schemapb"
)

func TestEmitCompiles(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
		schemapb.Str("wal_level"),
		schemapb.Enum("level").Values(map[int32]string{0: "minimal", 1: "replica"}).Required(),
		schemapb.Object("wal", schemapb.Bool("enabled").Required()),
	).MustBuild()

	f, err := model.Build(s, "myconfig")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Emit(f)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	src := string(out)
	for _, want := range []string{
		"package myconfig",
		"type InfraDiskV1 struct",
		"SharedBuffers int64 `json:\"shared_buffers\"`",
		"WalLevel *string `json:\"wal_level,omitempty\"`",
		"type InfraDiskV1_Level int32",
		"type InfraDiskV1_Wal struct",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("output missing %q\n---\n%s", want, src)
		}
	}
}
```

(Compilation of the emitted code is verified by golden tests in Task 10; here we
assert key fragments and that `go/format` accepts the output.)

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -v`
Expected: FAIL (undefined: Emit).

- [ ] **Step 3: Implement the orchestrator**

`emit.go`:

```go
// Package emit renders a model.File into gofmt'd Go source.
package emit

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

// Emit renders the file and runs it through gofmt.
func Emit(f *model.File) ([]byte, error) {
	var b bytes.Buffer
	writeHeader(&b, f)
	writeStructs(&b, f)
	writeEnums(&b, f)
	// sugar/roundtrip/schemawrap appended by later tasks:
	writeSchemaWrap(&b, f)
	writeRoundtrip(&b, f)
	writeSugar(&b, f)

	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt failed: %w\n--- raw ---\n%s", err, b.String())
	}
	return src, nil
}

func writeHeader(b *bytes.Buffer, f *model.File) {
	fmt.Fprintf(b, "// Code generated by schemapbgen. DO NOT EDIT.\n\n")
	fmt.Fprintf(b, "package %s\n\n", f.Package)
	fmt.Fprintf(b, "import (\n\t\"sync\"\n\t\"time\"\n\n")
	fmt.Fprintf(b, "\t\"google.golang.org/protobuf/proto\"\n")
	fmt.Fprintf(b, "\t\"google.golang.org/protobuf/types/known/structpb\"\n\n")
	fmt.Fprintf(b, "\t\"github.com/stroppy-io/schemapb/schemapb\"\n)\n\n")
	fmt.Fprintf(b, "var _ = time.Second // keep imports used\n\n")
}
```

(The `var _ = time.Second` guard is removed once roundtrip code that always uses
`time`/`proto`/`structpb` lands; harmless meanwhile.)

`structs.go`:

```go
package emit

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeStructs(b *bytes.Buffer, f *model.File) {
	for _, t := range f.Types {
		if t.Doc != "" {
			fmt.Fprintf(b, "// %s: %s\n", t.Name, t.Doc)
		}
		fmt.Fprintf(b, "type %s struct {\n", t.Name)
		for _, fld := range t.Fields {
			writeDoc(b, fld.Doc)
			tag := fld.JSONName
			if fld.OmitEmpty {
				tag += ",omitempty"
			}
			fmt.Fprintf(b, "\t%s %s `json:%q`\n", fld.Name, fld.GoType, tag)
		}
		fmt.Fprintf(b, "}\n\n")
		writeOneOfIfaces(b, t)
	}
}

func writeDoc(b *bytes.Buffer, doc string) {
	if doc == "" {
		return
	}
	for _, line := range splitLines(doc) {
		fmt.Fprintf(b, "\t// %s\n", line)
	}
}

func writeOneOfIfaces(b *bytes.Buffer, t *model.Type) {
	for _, fld := range t.Fields {
		if fld.OneOf == nil {
			continue
		}
		od := fld.OneOf
		fmt.Fprintf(b, "// %s is a discriminated union (discriminator %q).\n", od.IfaceName, od.Discriminator)
		fmt.Fprintf(b, "type %s interface{ is%s() }\n\n", od.IfaceName, od.IfaceName)
		// stable variant order
		keys := make([]string, 0, len(od.Variants))
		for k := range od.Variants {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(b, "func (*%s) is%s() {}\n", od.Variants[k], od.IfaceName)
		}
		fmt.Fprintf(b, "\n")
	}
}

func writeEnums(b *bytes.Buffer, f *model.File) {
	for _, e := range f.Enums {
		fmt.Fprintf(b, "type %s int32\n\n", e.Name)
		vals := make([]int, 0, len(e.Values))
		for k := range e.Values {
			vals = append(vals, int(k))
		}
		sort.Ints(vals)
		fmt.Fprintf(b, "const (\n")
		for _, v := range vals {
			label := e.Values[int32(v)]
			fmt.Fprintf(b, "\t%s%s %s = %d\n", e.Name, pascalLabel(label), e.Name, v)
		}
		fmt.Fprintf(b, ")\n\n")
		// String()
		fmt.Fprintf(b, "func (e %s) String() string {\n\tswitch e {\n", e.Name)
		for _, v := range vals {
			fmt.Fprintf(b, "\tcase %d:\n\t\treturn %q\n", v, e.Values[int32(v)])
		}
		fmt.Fprintf(b, "\tdefault:\n\t\treturn \"\"\n\t}\n}\n\n")
	}
}
```

`splitLines` and `pascalLabel` helpers — add to `structs.go`:

```go
func splitLines(s string) []string {
	var out, cur []rune
	_ = cur
	res := []string{}
	start := 0
	for i, r := range s {
		if r == '\n' {
			res = append(res, s[start:i])
			start = i + 1
		}
	}
	res = append(res, s[start:])
	_ = out
	return res
}

func pascalLabel(s string) string {
	// reuse model's rule: split on _-., PascalCase
	rs := []rune(s)
	if len(rs) == 0 {
		return "Unspecified"
	}
	// minimal inline PascalCase to avoid exporting model.pascal
	out := ""
	up := true
	for _, r := range rs {
		if r == '_' || r == '-' || r == '.' || r == ' ' {
			up = true
			continue
		}
		if up {
			out += string(toUpper(r))
			up = false
		} else {
			out += string(r)
		}
	}
	return out
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}
```

Add empty stub functions so `emit.go` compiles before later tasks fill them
(`writeSchemaWrap`, `writeRoundtrip`, `writeSugar`) — create
`cmd/schemapbgen/internal/emit/stubs.go`:

```go
package emit

import (
	"bytes"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

// Stubs replaced by Tasks 7-9.
func writeSchemaWrap(b *bytes.Buffer, f *model.File) {}
func writeRoundtrip(b *bytes.Buffer, f *model.File)  {}
func writeSugar(b *bytes.Buffer, f *model.File)       {}
```

- [ ] **Step 4: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schemapbgen/internal/emit/
git commit -m "feat(schemapbgen): emit structs, enums, oneof interfaces"
```

---

## Task 7: Emit — schema wrap (wire bytes, Schema(), Validate(), Identity)

**Files:**
- Create: `cmd/schemapbgen/internal/emit/schemawrap.go` (replaces the stub)
- Modify: `cmd/schemapbgen/internal/emit/stubs.go` (remove `writeSchemaWrap` stub)
- Test: `cmd/schemapbgen/internal/emit/emit_test.go`

- [ ] **Step 1: Add failing test**

Append to `emit_test.go`:

```go
func TestEmitSchemaWrap(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
	).MustBuild()
	f, _ := model.Build(s, "myconfig")
	out, err := Emit(f)
	if err != nil {
		t.Fatal(err)
	}
	src := string(out)
	for _, want := range []string{
		"var _schemaInfraDiskV1Wire = []byte{",
		"func (c *InfraDiskV1) Schema() *schemapb.Schema",
		"func (c *InfraDiskV1) Validate() []*schemapb.FieldError",
		"var InfraDiskV1Identity = &schemapb.SchemaIdentity{",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -run TestEmitSchemaWrap -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Remove the `writeSchemaWrap` stub from `stubs.go`. Create `schemawrap.go`:

```go
package emit

import (
	"bytes"
	"fmt"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeSchemaWrap(b *bytes.Buffer, f *model.File) {
	root := f.Root

	// wire bytes
	fmt.Fprintf(b, "var _schema%sWire = []byte{", root)
	for i, by := range f.Wire {
		if i%16 == 0 {
			fmt.Fprintf(b, "\n\t")
		}
		fmt.Fprintf(b, "0x%02x, ", by)
	}
	fmt.Fprintf(b, "\n}\n\n")

	// lazy decode
	fmt.Fprintf(b, "var (\n\t_schema%sOnce sync.Once\n\t_schema%sVal  *schemapb.Schema\n)\n\n", root, root)
	fmt.Fprintf(b, "func _schema%s() *schemapb.Schema {\n", root)
	fmt.Fprintf(b, "\t_schema%sOnce.Do(func() {\n", root)
	fmt.Fprintf(b, "\t\tvar s schemapb.Schema\n")
	fmt.Fprintf(b, "\t\tif err := proto.Unmarshal(_schema%sWire, &s); err != nil {\n\t\t\tpanic(err)\n\t\t}\n", root)
	fmt.Fprintf(b, "\t\t_schema%sVal = &s\n\t})\n\treturn _schema%sVal\n}\n\n", root, root)

	// Schema()
	fmt.Fprintf(b, "// Schema returns the schema this type was generated from.\n")
	fmt.Fprintf(b, "func (c *%s) Schema() *schemapb.Schema { return _schema%s() }\n\n", root, root)

	// Validate()
	fmt.Fprintf(b, "// Validate marshals the value and runs the schemapb engine against it.\n")
	fmt.Fprintf(b, "func (c *%s) Validate() []*schemapb.FieldError {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n")
	fmt.Fprintf(b, "\tif err != nil {\n\t\treturn []*schemapb.FieldError{schemapb.NewFieldError(\"\", err.Error())}\n\t}\n")
	fmt.Fprintf(b, "\treturn _schema%s().ValidateStruct(st)\n}\n\n", root)

	// Identity
	id := f.Identity
	fmt.Fprintf(b, "// %sIdentity is the schema identity this type was generated from.\n", root)
	fmt.Fprintf(b, "var %sIdentity = &schemapb.SchemaIdentity{Namespace: %q, Name: %q, Version: %q}\n\n",
		root, id.GetNamespace(), id.GetName(), id.GetVersion())
}
```

- [ ] **Step 4: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schemapbgen/internal/emit/
git commit -m "feat(schemapbgen): emit schema wrap — wire bytes, Schema(), Validate(), Identity"
```

---

## Task 8: Emit — roundtrip (ToValues / FromValues / ToFilled / ToBaked)

**Files:**
- Create: `cmd/schemapbgen/internal/emit/roundtrip.go` (replaces stub)
- Modify: `cmd/schemapbgen/internal/emit/stubs.go`
- Test: `cmd/schemapbgen/internal/emit/roundtrip_integration_test.go`

This emits a JSON-bridge roundtrip: the struct's `json` tags already mirror the
schema field names, so `ToValues` = `json.Marshal(c)` → `structpb` and
`FromValues` = `structpb` → JSON → `json.Unmarshal`. Computed fields carry
`omitempty` and are pointer/zero on output, so they naturally drop from
`ToValues` when unset. This keeps the generated code small and correct without
per-field marshalling.

- [ ] **Step 1: Add failing integration test**

`roundtrip_integration_test.go` writes a generated file to a temp dir, compiles
it in a throwaway module, and runs a roundtrip. To keep CI simple, instead assert
the emitted source contains the right signatures (full compile is covered by the
golden test in Task 10):

```go
package emit

import (
	"strings"
	"testing"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/schemapb"
)

func TestEmitRoundtripSignatures(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
	).MustBuild()
	f, _ := model.Build(s, "myconfig")
	out, _ := Emit(f)
	src := string(out)
	for _, want := range []string{
		"func (c *InfraDiskV1) ToValues() (*structpb.Struct, error)",
		"func FromValuesInfraDiskV1(st *structpb.Struct) (*InfraDiskV1, error)",
		"func (c *InfraDiskV1) ToFilled() (*schemapb.Filled, error)",
		"func (c *InfraDiskV1) ToBaked() (*schemapb.Baked, error)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -run TestEmitRoundtrip -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Remove the `writeRoundtrip` stub. Create `roundtrip.go`. Add `"encoding/json"`
to the generated header imports (modify `writeHeader` in `emit.go` to include
`"encoding/json"`).

```go
package emit

import (
	"bytes"
	"fmt"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeRoundtrip(b *bytes.Buffer, f *model.File) {
	root := f.Root

	fmt.Fprintf(b, "// ToValues marshals the value to a protobuf Struct (JSON-bridged).\n")
	fmt.Fprintf(b, "func (c *%s) ToValues() (*structpb.Struct, error) {\n", root)
	fmt.Fprintf(b, "\tj, err := json.Marshal(c)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\tst := &structpb.Struct{}\n\tif err := st.UnmarshalJSON(j); err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\treturn st, nil\n}\n\n")

	fmt.Fprintf(b, "// FromValues%s decodes a protobuf Struct into a new value.\n", root)
	fmt.Fprintf(b, "func FromValues%s(st *structpb.Struct) (*%s, error) {\n", root, root)
	fmt.Fprintf(b, "\tj, err := st.MarshalJSON()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\tvar c %s\n\tif err := json.Unmarshal(j, &c); err != nil {\n\t\treturn nil, err\n\t}\n", root)
	fmt.Fprintf(b, "\treturn &c, nil\n}\n\n")

	fmt.Fprintf(b, "// ToFilled wraps the value as a schemapb.Filled against this schema.\n")
	fmt.Fprintf(b, "func (c *%s) ToFilled() (*schemapb.Filled, error) {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\treturn &schemapb.Filled{Schema: &schemapb.SchemaRef{Source: &schemapb.SchemaRef_Schema{Schema: _schema%s()}}, Values: st}, nil\n}\n\n", root)

	fmt.Fprintf(b, "// ToBaked wraps the value as a schemapb.Baked (embedded schema + values).\n")
	fmt.Fprintf(b, "func (c *%s) ToBaked() (*schemapb.Baked, error) {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\treturn &schemapb.Baked{Schema: _schema%s(), Values: st}, nil\n}\n\n", root)
}
```

(Verify `SchemaRef_Schema` is the correct oneof wrapper name in `schema.pb.go`
before writing; adjust if the generated name differs.)

- [ ] **Step 4: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schemapbgen/internal/emit/
git commit -m "feat(schemapbgen): emit roundtrip (ToValues/FromValues/ToFilled/ToBaked)"
```

---

## Task 9: Emit — sugar (getters, builder, Clone, Default, MarshalJSON pass-through)

**Files:**
- Create: `cmd/schemapbgen/internal/emit/sugar.go` (replaces stub)
- Modify: `cmd/schemapbgen/internal/emit/stubs.go` (remove last stub)
- Test: `cmd/schemapbgen/internal/emit/emit_test.go`

- [ ] **Step 1: Add failing test**

Append to `emit_test.go`:

```go
func TestEmitSugar(t *testing.T) {
	s := schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
		schemapb.Str("wal_level"),
	).MustBuild()
	f, _ := model.Build(s, "myconfig")
	out, _ := Emit(f)
	src := string(out)
	for _, want := range []string{
		"func (c *InfraDiskV1) GetSharedBuffers() int64",
		"func (c *InfraDiskV1) GetWalLevel() string", // nil-safe deref of *string
		"func NewInfraDiskV1() *InfraDiskV1",
		"func (c *InfraDiskV1) WithSharedBuffers(v int64) *InfraDiskV1",
		"func (c *InfraDiskV1) Clone() *InfraDiskV1",
		"func DefaultInfraDiskV1() *InfraDiskV1",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -run TestEmitSugar -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Remove the `writeSugar` stub. Create `sugar.go`:

```go
package emit

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
)

func writeSugar(b *bytes.Buffer, f *model.File) {
	for _, t := range f.Types {
		writeGetters(b, t)
		writeBuilder(b, t)
		writeClone(b, t)
	}
	writeDefault(b, f)
}

// writeGetters emits nil-safe getters protobuf-style. For pointer fields the
// getter dereferences and returns the zero value when nil.
func writeGetters(b *bytes.Buffer, t *model.Type) {
	for _, fld := range t.Fields {
		if fld.OneOf != nil {
			continue // oneof read via type switch
		}
		base := strings.TrimPrefix(fld.GoType, "*")
		fmt.Fprintf(b, "func (c *%s) Get%s() %s {\n", t.Name, fld.Name, base)
		if fld.Pointer || strings.HasPrefix(fld.GoType, "[]") {
			fmt.Fprintf(b, "\tif c == nil || c.%s == nil {\n\t\tvar zero %s\n\t\treturn zero\n\t}\n", fld.Name, base)
			if fld.Pointer {
				fmt.Fprintf(b, "\treturn *c.%s\n}\n\n", fld.Name)
			} else {
				fmt.Fprintf(b, "\treturn c.%s\n}\n\n", fld.Name) // slice: return as-is
			}
		} else {
			fmt.Fprintf(b, "\tif c == nil {\n\t\tvar zero %s\n\t\treturn zero\n\t}\n\treturn c.%s\n}\n\n", base, fld.Name)
		}
	}
}

func writeBuilder(b *bytes.Buffer, t *model.Type) {
	fmt.Fprintf(b, "// New%s returns an empty %s for chained construction.\n", t.Name, t.Name)
	fmt.Fprintf(b, "func New%s() *%s { return &%s{} }\n\n", t.Name, t.Name, t.Name)
	for _, fld := range t.Fields {
		if fld.OneOf != nil {
			fmt.Fprintf(b, "func (c *%s) With%s(v %s) *%s { c.%s = v; return c }\n\n",
				t.Name, fld.Name, fld.GoType, t.Name, fld.Name)
			continue
		}
		base := strings.TrimPrefix(fld.GoType, "*")
		if fld.Pointer {
			fmt.Fprintf(b, "func (c *%s) With%s(v %s) *%s { c.%s = &v; return c }\n\n",
				t.Name, fld.Name, base, t.Name, fld.Name)
		} else {
			fmt.Fprintf(b, "func (c *%s) With%s(v %s) *%s { c.%s = v; return c }\n\n",
				t.Name, fld.Name, fld.GoType, t.Name, fld.Name)
		}
	}
}

// writeClone emits a JSON-bridge deep clone (correct for all generated shapes).
func writeClone(b *bytes.Buffer, t *model.Type) {
	fmt.Fprintf(b, "// Clone deep-copies the value via its JSON representation.\n")
	fmt.Fprintf(b, "func (c *%s) Clone() *%s {\n", t.Name, t.Name)
	fmt.Fprintf(b, "\tif c == nil {\n\t\treturn nil\n\t}\n")
	fmt.Fprintf(b, "\tj, _ := json.Marshal(c)\n\tvar out %s\n\t_ = json.Unmarshal(j, &out)\n\treturn &out\n}\n\n", t.Name)
}

// writeDefault emits Default<Root>() seeding schema defaults via the engine.
// It builds an empty struct, marshals to values, lets the engine apply defaults
// through validation's resolution, then reads back.
func writeDefault(b *bytes.Buffer, f *model.File) {
	root := f.Root
	fmt.Fprintf(b, "// Default%s returns a new value with schema defaults applied.\n", root)
	fmt.Fprintf(b, "func Default%s() *%s {\n", root, root)
	fmt.Fprintf(b, "\tc := &%s{}\n\tc.Default()\n\treturn c\n}\n\n", root)
	fmt.Fprintf(b, "// Default fills schema default values into empty fields of c.\n")
	fmt.Fprintf(b, "func (c *%s) Default() {\n", root)
	fmt.Fprintf(b, "\tst, err := c.ToValues()\n\tif err != nil {\n\t\treturn\n\t}\n")
	fmt.Fprintf(b, "\tm := st.AsMap()\n\t_schema%s().ApplyDefaults(m)\n", root)
	fmt.Fprintf(b, "\tns, err := structpb.NewStruct(m)\n\tif err != nil {\n\t\treturn\n\t}\n")
	fmt.Fprintf(b, "\tif got, err := FromValues%s(ns); err == nil {\n\t\t*c = *got\n\t}\n}\n\n", root)
}
```

> **Dependency note:** `writeDefault` calls `(*Schema).ApplyDefaults(map[string]any)`.
> If that method does not exist in the library, either (a) add it to the library
> as a thin wrapper that walks fields and fills `default` values into a map, or
> (b) simplify `Default()` to apply only top-level scalar defaults inline from
> the IR. Confirm during implementation; prefer (a) so nested defaults work.
> This is the one task that may require a small library-side addition.

- [ ] **Step 4: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./internal/emit/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/schemapbgen/internal/emit/
git commit -m "feat(schemapbgen): emit sugar — getters, builder, Clone, Default"
```

---

## Task 10: Parse + CLI Phase 1, golden compile test

**Files:**
- Create: `cmd/schemapbgen/internal/parse/parse.go`
- Modify: `cmd/schemapbgen/main.go`
- Create: `cmd/schemapbgen/internal/emit/testdata/disk.json`
- Test: `cmd/schemapbgen/internal/emit/golden_test.go`

- [ ] **Step 1: Implement parse**

`parse.go`:

```go
// Package parse loads a schemapb.Schema from protojson.
package parse

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
)

// FromJSONFile reads a protojson-encoded Schema from path.
func FromJSONFile(path string) (*schemapb.Schema, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s schemapb.Schema
	if err := protojson.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &s, nil
}
```

- [ ] **Step 2: Wire the generate command**

Replace `rootCmd` in `main.go` and add the generate command:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/emit"
	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/parse"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		in   []string
		out  string
		pkg  string
	)
	cmd := &cobra.Command{
		Use:   "schemapbgen",
		Short: "Generate typed Go structs from schemapb schemas",
		RunE: func(_ *cobra.Command, _ []string) error {
			if pkg == "" {
				return fmt.Errorf("-pkg is required")
			}
			for _, path := range in {
				s, err := parse.FromJSONFile(path)
				if err != nil {
					return err
				}
				f, err := model.Build(s, pkg)
				if err != nil {
					return err
				}
				src, err := emit.Emit(f)
				if err != nil {
					return err
				}
				dst := out
				if dst == "" || len(in) > 1 {
					dst = filepath.Join(filepath.Dir(path), f.Root+"_gen.go")
				}
				if err := os.WriteFile(dst, src, 0o644); err != nil {
					return err
				}
				fmt.Printf("wrote %s\n", dst)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&in, "in", nil, "input schema JSON file(s)")
	cmd.Flags().StringVar(&out, "out", "", "output Go file (single input only)")
	cmd.Flags().StringVar(&pkg, "pkg", "", "package name of generated code")
	_ = cmd.MarkFlagRequired("in")
	return cmd
}
```

- [ ] **Step 3: Create the golden fixture**

Generate `testdata/disk.json` by marshalling a representative schema. Add a
helper test that writes it on first run, or hand-author it. Minimal fixture:

```json
{
  "id": {"namespace": "infra", "name": "disk", "version": "v1"},
  "fields": [
    {"name": "shared_buffers", "required": true, "int64": {}},
    {"name": "wal_level", "string": {}},
    {"name": "level", "required": true, "enum": {"values": {"0": "minimal", "1": "replica"}}},
    {"name": "wal", "object": {"schema": {"fields": [{"name": "enabled", "required": true, "bool": {}}]}}}
  ]
}
```

- [ ] **Step 4: Write the golden compile test**

`golden_test.go`:

```go
package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/schemapb"
)

// TestGoldenCompiles generates code from disk.json, drops it into a temp module
// that requires the library via replace, and runs `go build` to prove the
// generated code compiles against the real runtime.
func TestGoldenCompiles(t *testing.T) {
	raw, err := os.ReadFile("testdata/disk.json")
	if err != nil {
		t.Fatal(err)
	}
	var s schemapb.Schema
	if err := protojson.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	f, err := model.Build(&s, "gen")
	if err != nil {
		t.Fatal(err)
	}
	src, err := Emit(f)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	repoRoot, _ := filepath.Abs("../../../..") // cmd/schemapbgen/internal/emit -> repo root
	must(t, os.WriteFile(filepath.Join(dir, "gen.go"), src, 0o644))
	gomod := "module gen\n\ngo 1.25.0\n\nrequire github.com/stroppy-io/schemapb v0.0.0\n\nreplace github.com/stroppy-io/schemapb => " + repoRoot + "\n"
	must(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))

	run(t, dir, "go", "mod", "tidy")
	run(t, dir, "go", "build", "./...")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
```

- [ ] **Step 5: Run the full suite**

Run: `cd cmd/schemapbgen && go test ./... -v`
Expected: PASS (including `TestGoldenCompiles` — generated code builds against the
real library). Fix any emit bug the compile surfaces (this is the task that
catches mismatched proto symbol names like `SchemaRef_Schema`).

- [ ] **Step 6: Commit**

```bash
git add cmd/schemapbgen/
git commit -m "feat(schemapbgen): parse protojson + CLI generate + golden compile test"
```

---

## Task 11: Phase 2 — generate from Go builder code (`-from-go-code`)

**Files:**
- Create: `cmd/schemapbgen/internal/parse/bridge.go`
- Modify: `cmd/schemapbgen/main.go` (add `-from-go-code` and `-symbol` flags)
- Test: `cmd/schemapbgen/internal/parse/bridge_test.go`

- [ ] **Step 1: Write the failing test**

`bridge_test.go` creates a tiny package exposing `func Provide() *schemapb.Schema`,
then asserts the bridge runs it and returns the schema:

```go
package parse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBridgeRunsProvider(t *testing.T) {
	dir := t.TempDir()
	repoRoot, _ := filepath.Abs("../../../..")
	pkg := `package prov

import "github.com/stroppy-io/schemapb/schemapb"

func Provide() *schemapb.Schema {
	return schemapb.NewSchema("infra", "disk", "v1").Fields(
		schemapb.Int64("shared_buffers").Required(),
	).MustBuild()
}
`
	os.WriteFile(filepath.Join(dir, "prov.go"), []byte(pkg), 0o644)
	gomod := "module prov\n\ngo 1.25.0\n\nrequire github.com/stroppy-io/schemapb v0.0.0\n\nreplace github.com/stroppy-io/schemapb => " + repoRoot + "\n"
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644)

	schemas, err := FromGoCode(dir, "Provide")
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 1 || schemas[0].GetId().GetName() != "disk" {
		t.Fatalf("got %+v", schemas)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd cmd/schemapbgen && go test ./internal/parse/ -run TestBridge -v`
Expected: FAIL (undefined: FromGoCode).

- [ ] **Step 3: Implement the bridge**

`bridge.go` — write a temp `main` into the user's module that imports the user
package, calls the symbol, and dumps protojson of each schema to stdout:

```go
package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
)

// FromGoCode compiles+runs the user's module at dir, calling the exported symbol
// (a func returning *schemapb.Schema or []*schemapb.Schema), and returns the
// schemas. The user's code must compile.
func FromGoCode(dir, symbol string) ([]*schemapb.Schema, error) {
	pkgPath, err := modulePath(dir)
	if err != nil {
		return nil, err
	}
	runnerDir, err := os.MkdirTemp(dir, "schemapbgen-runner-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(runnerDir)

	src := fmt.Sprintf(`package main

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	userpkg %q
)

func main() {
	v := userpkg.%s()
	var list []interface{ Reset() }
	_ = list
	dump := func(m interface{}) {
		b, err := protojson.Marshal(m.(interface{ Reset(); String() string; ProtoReflect() interface{} }).(protojsonMessage))
		_ = err
		fmt.Println(string(b))
	}
	_ = dump
	_ = v
	_ = os.Stdout
}
`, pkgPath, symbol)
	_ = src // see note below

	// Simpler, type-safe runner: assert the symbol's return to the known types.
	runner := fmt.Sprintf(`package main

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/schemapb/schemapb"
	userpkg %q
)

func main() {
	out := []*schemapb.Schema{}
	switch v := any(userpkg.%s()).(type) {
	case *schemapb.Schema:
		out = append(out, v)
	case []*schemapb.Schema:
		out = append(out, v...)
	default:
		panic("symbol must return *schemapb.Schema or []*schemapb.Schema")
	}
	for _, s := range out {
		b, err := protojson.Marshal(s)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%%s\n", b)
	}
}
`, pkgPath, symbol)

	if err := os.WriteFile(filepath.Join(runnerDir, "main.go"), []byte(runner), 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = runnerDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run provider: %w\n%s", err, stderr.String())
	}

	var schemas []*schemapb.Schema
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var rawObj json.RawMessage
		if err := dec.Decode(&rawObj); err != nil {
			return nil, err
		}
		var s schemapb.Schema
		if err := protojson.Unmarshal(rawObj, &s); err != nil {
			return nil, err
		}
		schemas = append(schemas, &s)
	}
	return schemas, nil
}

// modulePath returns the module path declared in dir/go.mod so the runner can
// import the user package.
func modulePath(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range bytes.Split(b, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("module ")) {
			return string(bytes.TrimSpace(line[len("module "):])), nil
		}
	}
	_ = build.Default
	return "", fmt.Errorf("no module path in %s/go.mod", dir)
}
```

Delete the dead first `src` block (it was scratch); keep only the typed `runner`.
The runner is written inside the user's module dir so `go run .` resolves the
user package and the library via the user's own go.mod.

- [ ] **Step 4: Add CLI flags**

In `main.go`, add `fromGoCode` and `symbol` flags; when `fromGoCode != ""`, call
`parse.FromGoCode` and loop the resulting schemas through model+emit (output file
per schema named `<Root>_gen.go` in `-out`'s dir or the current dir).

```go
	var fromGoCode, symbol string
	// ... inside RunE, before the JSON loop:
	if fromGoCode != "" {
		schemas, err := parse.FromGoCode(fromGoCode, symbol)
		if err != nil {
			return err
		}
		for _, s := range schemas {
			f, err := model.Build(s, pkg)
			if err != nil {
				return err
			}
			src, err := emit.Emit(f)
			if err != nil {
				return err
			}
			dst := f.Root + "_gen.go"
			if out != "" {
				dst = filepath.Join(filepath.Dir(out), dst)
			}
			if err := os.WriteFile(dst, src, 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s\n", dst)
		}
		return nil
	}
	// ... existing JSON loop ...
```

```go
	cmd.Flags().StringVar(&fromGoCode, "from-go-code", "", "dir of a Go module exposing a schema provider func")
	cmd.Flags().StringVar(&symbol, "symbol", "", "exported provider func returning *schemapb.Schema or []*schemapb.Schema")
```

And relax `MarkFlagRequired("in")`: make `in` required only when `from-go-code`
is empty (validate inside `RunE` instead of `MarkFlagRequired`).

- [ ] **Step 5: Run, verify pass**

Run: `cd cmd/schemapbgen && go test ./... -v`
Expected: PASS (`TestBridgeRunsProvider` runs the provider and returns the disk
schema).

- [ ] **Step 6: Commit**

```bash
git add cmd/schemapbgen/
git commit -m "feat(schemapbgen): Phase 2 -from-go-code (compile+run builder, dump schema)"
```

---

## Task 12: Docs + Makefile target + go:generate example

**Files:**
- Create: `cmd/schemapbgen/README.md`
- Modify: `Makefile`

- [ ] **Step 1: Write the README**

`cmd/schemapbgen/README.md` documenting: install, Phase 1 (`-in/-out/-pkg`),
Phase 2 (`//go:generate go run github.com/stroppy-io/schemapb/cmd/schemapbgen
-from-go-code . -symbol Provide -pkg myconfig`), the naming scheme, and the
generated API surface (Validate/ToValues/FromValues/Default/getters/builder).

- [ ] **Step 2: Add a Makefile target**

Append to `Makefile`:

```make
.PHONY: schemapbgen-test
schemapbgen-test:
	cd cmd/schemapbgen && go test ./...
```

- [ ] **Step 3: Run**

Run: `make schemapbgen-test`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/schemapbgen/README.md Makefile
git commit -m "docs(schemapbgen): README + make target"
```

---

## Self-review notes

- **Spec coverage:** §3 type map → Tasks 4–5. §3 pointer rules → Task 4. §4 naming
  → Task 2 + Child usage in 4/5. §5 oneof iface → Task 5 (IR) + Task 6 (emit). §6
  computed → Task 5 (omitempty/Computed) + Task 8 (ToValues omits via omitempty).
  §7 sugar → Tasks 7–9. §8 runtime dep → Task 1 + golden compile Task 10. §9 CLI
  Phase 1 → Task 10, Phase 2 → Task 11. §11 testing → golden (10), roundtrip
  signatures (8), collision (4), bridge (11).
- **Open implementation confirmations (resolve at execution):**
  1. `SchemaRef_Schema` / `Schema_Filed_Uint32` exact generated wrapper names —
     verify against `schema.pb.go` (Task 8/5).
  2. `(*Schema).ApplyDefaults` may not exist — Task 9 note gives the fallback
     (add a small library method, or inline top-level defaults).
  3. Computed pointer-vs-value: Task 5 makes computed optional→pointer; revisit if
     a required computed should be a value.
- **Known simplification (logged, not silent):** roundtrip uses a JSON bridge
  (struct↔JSON↔structpb), relying on `json` tags matching schema field names.
  `time.Duration`/`time.Time` JSON encoding must match what the engine expects
  (the engine parses duration/timestamp strings — see `checkDuration`/
  `checkTimestamp` in `validate.go`); Task 10's golden compile + a roundtrip
  assertion should confirm the string formats line up, else add custom
  MarshalJSON for those two types.
```
