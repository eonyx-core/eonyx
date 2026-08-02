package license

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// canonicalJSON reproduces the license server's canonical form, byte-compatible
// with Python's json.dumps(obj, sort_keys=True, separators=(",", ":")) with
// ensure_ascii=True. Both license files and server responses are Ed25519-signed
// over these exact bytes, so the serialization must not change.
func canonicalJSON(v any, b *strings.Builder) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		encodeCanonString(x, b)
	case int:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case float64:
		// encoding/json decodes every number as float64; the protocol allows
		// integers only.
		if math.IsNaN(x) || math.IsInf(x, 0) || x != math.Trunc(x) {
			return fmt.Errorf("non-integer number in payload: %v", x)
		}
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := canonicalJSON(e, b); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			encodeCanonString(k, b)
			b.WriteByte(':')
			if err := canonicalJSON(x[k], b); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("unsupported type in payload: %T", v)
	}
	return nil
}

// encodeCanonString escapes like Python's ensure_ascii: short escapes for the
// usual control characters, printable ASCII verbatim, and \uXXXX (UTF-16 code
// units, so surrogate pairs for astral characters) for everything else.
func encodeCanonString(s string, b *strings.Builder) {
	b.WriteByte('"')
	for _, u := range utf16.Encode([]rune(s)) {
		switch u {
		case 8:
			b.WriteString(`\b`)
		case 9:
			b.WriteString(`\t`)
		case 10:
			b.WriteString(`\n`)
		case 12:
			b.WriteString(`\f`)
		case 13:
			b.WriteString(`\r`)
		case 34:
			b.WriteString(`\"`)
		case 92:
			b.WriteString(`\\`)
		default:
			if u >= 0x20 && u <= 0x7e {
				b.WriteByte(byte(u))
			} else {
				fmt.Fprintf(b, `\u%04x`, u)
			}
		}
	}
	b.WriteByte('"')
}

// signableBytes returns the canonical bytes that get signed: the payload with
// its "signature" field removed.
func signableBytes(payload map[string]any) ([]byte, error) {
	filtered := make(map[string]any, len(payload))
	for k, v := range payload {
		if k != "signature" {
			filtered[k] = v
		}
	}
	var b strings.Builder
	if err := canonicalJSON(filtered, &b); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
