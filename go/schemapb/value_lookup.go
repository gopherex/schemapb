// Value path lookup: resolving a path in the ValidationError dialect
// ("replicas[0].name", "tablespaces.main.location") to the value it
// addresses inside a StructValue — the path from a validation error
// fetches the offending value directly.
//
// The string form cannot address a map key containing '.' or '[' (the same
// ambiguity error paths carry); the Field/Index steppers cover arbitrary
// keys without parsing.

package schemapb

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueLookupReason classifies why a value path failed to resolve. The
// values are stable spec strings shared by all implementations.
type ValueLookupReason string

const (
	// ValueLookupEmptyPath - the path has no segments.
	ValueLookupEmptyPath ValueLookupReason = "empty_path"
	// ValueLookupBadPath - the path is not in the error-path dialect.
	ValueLookupBadPath ValueLookupReason = "bad_path"
	// ValueLookupNotFound - the struct at At has no field Segment.
	ValueLookupNotFound ValueLookupReason = "not_found"
	// ValueLookupIndexOutOfRange - the list at At is shorter than the index.
	ValueLookupIndexOutOfRange ValueLookupReason = "index_out_of_range"
	// ValueLookupNotAStruct - a key segment was applied to a non-struct.
	ValueLookupNotAStruct ValueLookupReason = "not_a_struct"
	// ValueLookupNotAList - an index segment was applied to a non-list.
	ValueLookupNotAList ValueLookupReason = "not_a_list"
)

// ValueLookupError pinpoints the failing segment of a value path: At is
// the resolved parent path ("" for root), Segment the key or "[i]" index
// that failed.
type ValueLookupError struct {
	At      string
	Segment string
	Reason  ValueLookupReason
}

// lookupRoot names the root scope in lookup error texts.
const lookupRoot = "root"

func (e *ValueLookupError) Error() string {
	where := lookupRoot
	if e.At != "" {
		where = fmt.Sprintf("%q", e.At)
	}

	switch e.Reason {
	case ValueLookupEmptyPath:
		return "schemapb: value lookup: empty path"
	case ValueLookupBadPath:
		return fmt.Sprintf("schemapb: value lookup: malformed path %q", e.Segment)
	case ValueLookupNotFound:
		return fmt.Sprintf("schemapb: value lookup: no field %q in %s", e.Segment, where)
	case ValueLookupIndexOutOfRange:
		return fmt.Sprintf("schemapb: value lookup: index %s out of range in %s", e.Segment, where)
	case ValueLookupNotAStruct:
		return fmt.Sprintf("schemapb: value lookup: %s is not a struct, cannot read field %q", where, e.Segment)
	default: // ValueLookupNotAList
		return fmt.Sprintf("schemapb: value lookup: %s is not a list, cannot index %s", where, e.Segment)
	}
}

// Field steps into a struct value member; nil when the value is not a
// struct or has no such field. Handles keys the string path cannot spell.
func (v *Value) Field(name string) *Value {
	return v.GetStructValue().GetFields()[name]
}

// Index steps into a list value element; nil when the value is not a list
// or the index is out of range.
func (v *Value) Index(i int) *Value {
	items := v.GetListValue().GetItems()
	if i < 0 || i >= len(items) {
		return nil
	}

	return items[i]
}

// Field returns a top-level member of the struct (nil when absent).
func (s *StructValue) Field(name string) *Value {
	return s.GetFields()[name]
}

// pathToken is one parsed step: a struct key or a list index.
type pathToken struct {
	key   string
	index int
	isKey bool
}

// parseValuePath tokenizes the error-path dialect: key ('.' key | '[' int ']')*.
func parseValuePath(path string) ([]pathToken, bool) {
	var tokens []pathToken

	rest := path

	for rest != "" {
		switch rest[0] {
		case '[':
			if len(tokens) == 0 {
				return nil, false // paths start with a key, not an index
			}

			idx, rem, ok := parseIndex(rest)
			if !ok {
				return nil, false
			}

			tokens = append(tokens, pathToken{index: idx})
			rest = rem
		case '.':
			if len(tokens) == 0 {
				return nil, false // leading dot
			}

			rest = rest[1:]

			// A dot must be followed by a key: no trailing dot, no "a..b",
			// no "a.[0]".
			if rest == "" || rest[0] == '.' || rest[0] == '[' {
				return nil, false
			}
		default:
			end := strings.IndexAny(rest, ".[")
			if end < 0 {
				end = len(rest)
			}

			tokens = append(tokens, pathToken{key: rest[:end], isKey: true})
			rest = rest[end:]
		}
	}

	return tokens, len(tokens) > 0
}

// parseIndex consumes one "[digits]" token and requires ".", "[" or the
// end after it.
func parseIndex(rest string) (int, string, bool) {
	end := strings.IndexByte(rest, ']')
	if end < 2 || rest[1] < '0' || rest[1] > '9' { // digits only: no "+3", "-0", "[]"
		return 0, "", false
	}

	idx, err := strconv.Atoi(rest[1:end])
	if err != nil || idx < 0 {
		return 0, "", false
	}

	rem := rest[end+1:]
	if rem != "" && rem[0] != '.' && rem[0] != '[' {
		return 0, "", false
	}

	return idx, rem, true
}

// Lookup resolves a path in the ValidationError dialect against the
// struct's values. It returns the addressed value, or a *ValueLookupError
// naming the exact segment that failed.
func (s *StructValue) Lookup(path string) (*Value, error) {
	if path == "" {
		return nil, &ValueLookupError{Reason: ValueLookupEmptyPath}
	}

	tokens, ok := parseValuePath(path)
	if !ok {
		return nil, &ValueLookupError{Segment: path, Reason: ValueLookupBadPath}
	}

	cur := StructV(s.GetFields())
	parent := ""

	for _, tok := range tokens {
		if tok.isKey {
			m, isStruct := cur.AsStruct()
			if !isStruct {
				return nil, &ValueLookupError{At: parent, Segment: tok.key, Reason: ValueLookupNotAStruct}
			}

			next, found := m[tok.key]
			if !found {
				return nil, &ValueLookupError{At: parent, Segment: tok.key, Reason: ValueLookupNotFound}
			}

			cur = next
			parent = joinPath(parent, tok.key)

			continue
		}

		seg := fmt.Sprintf("[%d]", tok.index)

		items, isList := cur.AsList()
		if !isList {
			return nil, &ValueLookupError{At: parent, Segment: seg, Reason: ValueLookupNotAList}
		}

		if tok.index >= len(items) {
			return nil, &ValueLookupError{At: parent, Segment: seg, Reason: ValueLookupIndexOutOfRange}
		}

		cur = items[tok.index]
		parent += seg
	}

	return cur, nil
}
