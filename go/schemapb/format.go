package schemapb

import (
	"maps"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"time"
)

// FormatFunc reports whether s conforms to one named string format.
type FormatFunc func(s string) bool

// FormatRegistry maps format registry identifiers (String.format) to their
// checkers. The spec's core formats are always present; extensions are added
// per engine with WithFormats. A format the engine does not know fails
// validation with ERROR_CODE_UNSUPPORTED_FORMAT — never a silent pass.
type FormatRegistry map[string]FormatFunc

var (
	uuidRE     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$`)
	hostnameRE = regexp.MustCompile(`(?i)^([a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)
)

// CoreFormats returns a fresh registry with the spec's mandatory core formats.
func CoreFormats() FormatRegistry {
	return FormatRegistry{
		FormatEmail: func(s string) bool {
			_, err := mail.ParseAddress(s)
			return err == nil
		},
		FormatURL: func(s string) bool {
			_, err := url.ParseRequestURI(s)
			return err == nil
		},
		FormatUUID: uuidRE.MatchString,
		FormatIPv4: func(s string) bool {
			addr, err := netip.ParseAddr(s)
			return err == nil && addr.Is4()
		},
		FormatIPv6: func(s string) bool {
			addr, err := netip.ParseAddr(s)
			return err == nil && addr.Is6() && !addr.Is4In6()
		},
		FormatIP: func(s string) bool {
			_, err := netip.ParseAddr(s)
			return err == nil
		},
		FormatHostname: func(s string) bool {
			return len(s) <= 253 && hostnameRE.MatchString(s)
		},
		FormatDate: func(s string) bool {
			_, err := time.Parse(time.DateOnly, s)
			return err == nil
		},
		FormatTime: func(s string) bool {
			_, err := time.Parse(time.TimeOnly, s)
			return err == nil
		},
		FormatDatetime: func(s string) bool {
			_, err := time.Parse(time.RFC3339, s)
			return err == nil
		},
	}
}

// CompileOption customises Compile.
type CompileOption func(*compileConfig)

type compileConfig struct {
	formats FormatRegistry
}

// WithFormats adds (or overrides) format checkers on top of the core
// registry. Use namespaced identifiers for extensions ("k8s.quantity").
func WithFormats(reg FormatRegistry) CompileOption {
	return func(c *compileConfig) {
		maps.Copy(c.formats, reg)
	}
}
