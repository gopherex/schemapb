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
	// optional scalar -> pointer (GoType includes the * prefix)
	assertField(t, root, "WalLevel", "*string", true)
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
	assertField(t, root, "Level", "InfraDiskV1_Level", false) // enum named type
	assertField(t, root, "Tags", "[]string", false)           // list of scalar
	assertField(t, root, "Ttl", "time.Duration", false)
	assertField(t, root, "At", "time.Time", false)
	assertField(t, root, "Parent", "*InfraDiskV1_Node", true)     // ref -> pointer to def
	assertField(t, root, "Storage", "InfraDiskV1_Storage", false) // oneof iface

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

func findType(f *File, name string) *Type {
	for _, t := range f.Types {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// TestBuildListOfObject locks the list-element-from-items[0] mapping: an object
// item produces a []<Parent>_<Field>Item slice of a generated struct, and the
// element struct carries the object's fields.
func TestBuildListOfObject(t *testing.T) {
	s := schemapb.NewSchema("infra", "node", "v1").Fields(
		schemapb.List("peers", schemapb.Object("peer",
			schemapb.Str("host").Required(),
			schemapb.Int32("weight"),
		)).Required(),
	).MustBuild()

	f, err := Build(s, "p")
	if err != nil {
		t.Fatal(err)
	}
	root := findType(f, "InfraNodeV1")
	assertField(t, root, "Peers", "[]InfraNodeV1_PeersItem", false)
	item := findType(f, "InfraNodeV1_PeersItem")
	if item == nil {
		t.Fatal("element struct InfraNodeV1_PeersItem missing")
	}
	assertField(t, item, "Host", "string", false)  // required scalar
	assertField(t, item, "Weight", "*int32", true) // optional scalar
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
