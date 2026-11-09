package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const PasswordMin = 8
const PasswordMax = 128

// 64 MiB, 3 passes, 2 lanes; parameters are persisted per hash for future upgrades.
const argonMemory = 64 * 1024
const argonTime = 3
const argonThreads = 2

var weakPasswords = map[string]bool{}

func init() {
	for _, p := range strings.Fields("password password1 password123 password1234 12345678 123456789 1234567890 12345678910 123456789a 123456abcd qwertyui qwertyuiop qwerty123 qwerty1234 abcdefgh abcdefghij abc12345 abc123456 admin123 admin1234 administrator iloveyou welcome1 welcome123 letmein123 changeme changeme123 password! passw0rd passw0rd! 1qaz2wsx 1q2w3e4r 1q2w3e4r5t 87654321 infrapro infrapro123") {
		weakPasswords[p] = true
	}
}
func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || n < PasswordMin || n > PasswordMax {
		return FieldError("password", "密码需为 8–128 个字符，空格和中文均计入长度。")
	}
	normalized := strings.ToLower(strings.TrimSpace(password))
	allSame := true
	runes := []rune(normalized)
	for i := 1; i < len(runes); i++ {
		if runes[i] != runes[0] {
			allSame = false
			break
		}
	}
	if weakPasswords[normalized] || allSame {
		return FieldError("password", "请使用不常见且不重复的长口令。")
	}
	return nil
}
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, 32)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, enc.EncodeToString(salt), enc.EncodeToString(hash)), nil
}
func VerifyPassword(encoded, password string) bool {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) > PasswordMax {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil || memory < 19*1024 || memory > 256*1024 || iterations < 2 || iterations > 10 || threads < 1 || threads > 8 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func digest(value string) string { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
func validToken(value string) bool {
	b, e := base64.RawURLEncoding.DecodeString(value)
	return e == nil && len(b) == 32
}
