package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/schemapb/cmd/schemapbgen/internal/model"
	"github.com/stroppy-io/schemapb/schemapb"
)

// runOut runs a command in dir and returns its combined output, failing on error.
func runOut(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}

// TestRoundtripRuntime generates code for a schema exercising duration, a
// discriminated oneof, a list-of-object, a ref, and a scalar list, then
// compiles AND runs a program that validates and round-trips a value. This
// locks the two engine-contract fixes: duration serialized as a string, and
// oneof serialized as a single object carrying the discriminator. It also locks
// the list-element-from-items[0] mapping (a list of objects).
func TestRoundtripRuntime(t *testing.T) {
	s := schemapb.NewSchema("infra", "node", "v1").
		Def("addr", schemapb.Str("host").Required(), schemapb.Int32("port").Required()).
		Fields(
			schemapb.Str("name").Required(),
			schemapb.Duration("grace"),
			schemapb.Ref("primary", "addr"),
			schemapb.List("tags", schemapb.Str("v")),
			schemapb.List("ttls", schemapb.Duration("d")),
			schemapb.List("peers", schemapb.Object("peer",
				schemapb.Str("host").Required(),
				schemapb.Int32("weight"),
			)),
			schemapb.OneOf("storage", "kind").
				Variant("local", schemapb.Str("path").Required()).
				Variant("s3", schemapb.Str("bucket").Required()),
			schemapb.List("routes", schemapb.OneOf("r", "kind").
				Variant("http", schemapb.Str("path").Required()).
				Variant("tcp", schemapb.Int32("port").Required())),
		).MustBuild()

	f, err := model.Build(s, "main")
	if err != nil {
		t.Fatal(err)
	}
	src, err := Emit(f)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	repoRoot, _ := filepath.Abs("../../../..")
	must(t, os.WriteFile(filepath.Join(dir, "gen.go"), src, 0o644))
	gomod := "module gen\n\ngo 1.25.0\n\nrequire github.com/stroppy-io/schemapb v0.0.0\n\nreplace github.com/stroppy-io/schemapb => " + repoRoot + "\n"
	must(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))

	main := `package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	g := 30 * time.Second
	w := int32(5)
	c := NewInfraNodeV1().WithName("n1")
	c.Grace = &g
	c.Primary = &InfraNodeV1_Addr{Host: "h", Port: 5432}
	c.Tags = []string{"a", "b"}
	c.Peers = []InfraNodeV1_PeersItem{{Host: "p1", Weight: &w}}
	c.Ttls = []time.Duration{2 * time.Minute, 3 * time.Second}
	c.Storage = &InfraNodeV1_Storage_Local{Path: "/data"}
	c.Routes = []InfraNodeV1_RoutesItem{
		&InfraNodeV1_RoutesItem_Http{Path: "/api"},
		&InfraNodeV1_RoutesItem_Tcp{Port: 8080},
	}

	if errs := c.Validate(); len(errs) != 0 {
		for _, e := range errs {
			fmt.Println("VALIDATE_ERR", e.GetField(), e.GetMessage())
		}
		os.Exit(1)
	}
	st, err := c.ToValues()
	if err != nil {
		fmt.Println("TOVALUES_ERR", err)
		os.Exit(1)
	}
	back, err := FromValuesInfraNodeV1(st)
	if err != nil {
		fmt.Println("FROMVALUES_ERR", err)
		os.Exit(1)
	}
	loc, ok := back.Storage.(*InfraNodeV1_Storage_Local)
	if !ok || loc.Path != "/data" {
		fmt.Printf("ONEOF_BAD %T\n", back.Storage)
		os.Exit(1)
	}
	if back.GetGrace() != 30*time.Second {
		fmt.Println("DURATION_BAD", back.GetGrace())
		os.Exit(1)
	}
	if len(back.Peers) != 1 || back.Peers[0].Host != "p1" || back.Peers[0].Weight == nil || *back.Peers[0].Weight != 5 {
		fmt.Println("LIST_BAD")
		os.Exit(1)
	}
	if back.Primary.GetHost() != "h" {
		fmt.Println("REF_BAD")
		os.Exit(1)
	}
	if len(back.Ttls) != 2 || back.Ttls[0] != 2*time.Minute || back.Ttls[1] != 3*time.Second {
		fmt.Println("DURLIST_BAD", back.Ttls)
		os.Exit(1)
	}
	if len(back.Routes) != 2 {
		fmt.Println("ONEOFLIST_LEN_BAD", len(back.Routes))
		os.Exit(1)
	}
	h, ok := back.Routes[0].(*InfraNodeV1_RoutesItem_Http)
	if !ok || h.Path != "/api" {
		fmt.Printf("ONEOFLIST_0_BAD %T\n", back.Routes[0])
		os.Exit(1)
	}
	tcp, ok := back.Routes[1].(*InfraNodeV1_RoutesItem_Tcp)
	if !ok || tcp.Port != 8080 {
		fmt.Printf("ONEOFLIST_1_BAD %T\n", back.Routes[1])
		os.Exit(1)
	}
	fmt.Println("ALL_OK")
}
`
	must(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644))

	run(t, dir, "go", "mod", "tidy")
	out := runOut(t, dir, "go", "run", ".")
	if !strings.Contains(out, "ALL_OK") {
		t.Fatalf("generated program did not pass:\n%s", out)
	}
}
