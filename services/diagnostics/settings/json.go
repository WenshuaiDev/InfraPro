package settings

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
)

type Error string

func (e Error) Error() string { return string(e) }

const Invalid Error = "configuration_invalid"

func Code(err error) string {
	var e Error
	if errors.As(err, &e) {
		return string(e)
	}
	return "configuration_invalid"
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func ValidID(v string) bool { return idPattern.MatchString(v) }
func Decimal(v string, zero bool) bool {
	n, e := strconv.ParseUint(v, 10, 64)
	return e == nil && strconv.FormatUint(n, 10) == v && (zero || n > 0)
}
func Number(v string) uint64 { n, _ := strconv.ParseUint(v, 10, 64); return n }
func Next(v string) (string, error) {
	n := Number(v)
	if n == ^uint64(0) {
		return "", Invalid
	}
	return strconv.FormatUint(n+1, 10), nil
}
func Name(v string) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) < 1 || utf8.RuneCountInString(v) > 80 {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func OneOf(v string, values ...string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func Digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// Strict rejects duplicate keys as well as unknown fields, trailing documents,
// deep objects and malformed UTF-8. Standard encoding/json alone accepts duplicates.
func Strict(data []byte, dst any) error {
	if !utf8.Valid(data) {
		return Invalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 12 {
			return Invalid
		}
		t, e := d.Token()
		if e != nil {
			return Invalid
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return Invalid
					}
					s, ok := k.(string)
					if !ok || seen[s] {
						return Invalid
					}
					seen[s] = true
					if e = walk(depth + 1); e != nil {
						return e
					}
				}
			case '[':
				for d.More() {
					if e := walk(depth + 1); e != nil {
						return e
					}
				}
			default:
				return Invalid
			}
			_, e = d.Token()
			if e != nil {
				return Invalid
			}
		}
		return nil
	}
	if e := walk(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return Invalid
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		return Invalid
	}
	return nil
}
func Load(path string, dst any) (string, error) {
	data, e := securefile.Read(path, 4<<20, securefile.Private())
	if e != nil {
		return "", Invalid
	}
	if e = Strict(data, dst); e != nil {
		return "", e
	}
	return Digest(data), nil
}
func Save(path string, v any, exclusive bool) error {
	data, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return Invalid
	}
	return securefile.Atomic(path, append(data, '\n'), securefile.Private(), exclusive)
}
