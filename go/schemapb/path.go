package schemapb

import (
	"strconv"
	"strings"
)

// pathSegments decodes the canonical, escaped internal path, never an ambiguous
// concatenation of raw map keys. Simple legacy field paths retain their spelling.
//
//nolint:gocognit,nestif // small lexer tracks quoted keys and bracketed indices
func pathSegments(path string) []*PathSegment {
	var out []*PathSegment

	for path != "" {
		if path[0] == '.' {
			path = path[1:]
			continue
		}

		if path[0] != '[' {
			end := strings.IndexAny(path, ".[")
			if end < 0 {
				end = len(path)
			}

			out = append(out, &PathSegment{Segment: &PathSegment_Key{Key: path[:end]}})
			path = path[end:]

			continue
		}

		path = path[1:]
		if strings.HasPrefix(path, "\"") {
			end := 1
			for end < len(path) {
				if path[end] == '\\' {
					end += 2
					continue
				}

				if path[end] == '"' {
					break
				}

				end++
			}

			if end >= len(path) {
				return nil
			}

			key, err := strconv.Unquote(path[:end+1])
			if err != nil || end+1 >= len(path) || path[end+1] != ']' {
				return nil
			}

			out = append(out, &PathSegment{Segment: &PathSegment_Key{Key: key}})
			path = path[end+2:]
		} else {
			end := strings.IndexByte(path, ']')
			if end < 0 {
				return nil
			}

			index, err := strconv.ParseUint(path[:end], 10, 64)
			if err != nil {
				return nil
			}

			out = append(out, &PathSegment{Segment: &PathSegment_Index{Index: index}})
			path = path[end+1:]
		}
	}

	return out
}

func completeErrorPaths(result *ValidationResult) {
	for _, err := range result.GetErrors() {
		err.PathSegments = pathSegments(err.Path)
	}
}
