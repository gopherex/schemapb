package schemapb

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var jsonNumberRE = regexp.MustCompile(`^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$`)

// jsonInteger converts decimal notation exactly without allocating powers of ten
// proportional to an untrusted exponent. Only up to 20 significant digits can fit.
func jsonInteger(number json.Number) (string, bool) {
	raw := string(number)
	if !jsonNumberRE.MatchString(raw) {
		return "", false
	}

	negative := strings.HasPrefix(raw, "-")
	raw = strings.TrimPrefix(raw, "-")
	mantissa, exponent, hasExp := strings.Cut(strings.ToLower(raw), "e")
	integer, fraction, _ := strings.Cut(mantissa, ".")

	digits := strings.TrimLeft(integer+fraction, "0")
	if digits == "" {
		return "0", true
	}

	exp := int64(0)

	if hasExp {
		var err error

		exp, err = strconv.ParseInt(exponent, 10, 64)
		if err != nil || exp > int64(len(raw))+20 || exp < -int64(len(raw))-20 {
			return "", false
		}
	}

	scale := exp - int64(len(fraction))
	for scale < 0 && strings.HasSuffix(digits, "0") {
		digits = strings.TrimSuffix(digits, "0")
		scale++
	}

	if scale < 0 || int64(len(digits))+scale > 20 {
		return "", false
	}

	digits += strings.Repeat("0", int(scale))
	if negative {
		digits = "-" + digits
	}

	return digits, true
}

// JSON numbers are numeric input, independent of the schema's string Coerce flag.
//
//nolint:cyclop // kind-specific range and precision selection
func jsonNumberInput(f *Schema_Field, value any) (any, bool) {
	number, ok := value.(json.Number)
	if !ok || !jsonNumberRE.MatchString(string(number)) {
		return value, false
	}

	switch {
	case f.GetInt32() != nil || f.GetInt64() != nil:
		raw, integral := jsonInteger(number)
		if !integral {
			return value, false
		}

		bits := 64
		if f.GetInt32() != nil {
			bits = 32
		}

		n, err := strconv.ParseInt(raw, 10, bits)

		return n, err == nil
	case f.GetUint32() != nil || f.GetUint64() != nil:
		raw, integral := jsonInteger(number)
		if !integral {
			return value, false
		}

		bits := 64
		if f.GetUint32() != nil {
			bits = 32
		}

		n, err := strconv.ParseUint(raw, 10, bits)

		return n, err == nil
	case f.GetFloat() != nil || f.GetDouble() != nil:
		bits := 64
		if f.GetFloat() != nil {
			bits = 32
		}

		n, err := strconv.ParseFloat(string(number), bits)

		return n, err == nil && !math.IsInf(n, 0)
	default:
		return value, false
	}
}
