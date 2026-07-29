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
