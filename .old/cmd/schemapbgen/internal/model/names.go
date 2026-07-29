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
