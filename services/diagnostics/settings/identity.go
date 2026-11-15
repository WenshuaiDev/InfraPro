package settings

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

func PasswordHash(password string) (string, error) {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 128 {
		return "", Invalid
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", Invalid
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return "$argon2id$v=19$m=65536,t=3,p=2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash), nil
}
func hashParts(encoded string) ([]byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=2" {
		return nil, nil, Invalid
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[4])
	if e != nil || len(salt) != 16 {
		return nil, nil, Invalid
	}
	hash, e := base64.RawStdEncoding.DecodeString(parts[5])
	if e != nil || len(hash) != 32 {
		return nil, nil, Invalid
	}
	return salt, hash, nil
}
func VerifyPassword(password, encoded string) bool {
	salt, want, e := hashParts(encoded)
	if e != nil || len(password) > 128 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func (a Auth) Validate() error {
	if a.SchemaVersion != 1 || !Decimal(a.Revision, false) || a.Users == nil || len(a.Users) > 32 {
		return Invalid
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, u := range a.Users {
		if !ValidID(u.ID) || ids[u.ID] || !usernamePattern.MatchString(u.Username) || names[u.Username] || len(u.Permissions) != 1 || u.Permissions[0] != "diagnostics.read" {
			return Invalid
		}
		if _, _, e := hashParts(u.PasswordHash); e != nil {
			return e
		}
		ids[u.ID] = true
		names[u.Username] = true
	}
	return nil
}
func (k Keyring) Validate(m Manifest, localNode string) error {
	if k.SchemaVersion != 1 || k.InstanceID != m.InstanceID || !Decimal(k.Revision, false) || k.Keys == nil || len(k.Keys) > 1024 {
		return Invalid
	}
	ids, active := map[string]bool{}, map[string]bool{}
	for _, v := range k.Keys {
		n, ok := m.Node(v.NodeID)
		digest, e := hex.DecodeString(v.SecretSHA256)
		if !ok || v.NodeID == localNode || !ValidID(v.KeyID) || ids[v.KeyID] || !Decimal(v.Generation, false) || !Decimal(v.AssignmentRevision, false) || e != nil || len(digest) != 32 || hex.EncodeToString(digest) != v.SecretSHA256 {
			return Invalid
		}
		if v.Enabled {
			if active[v.NodeID] || !n.Enabled || v.AssignmentRevision != n.AssignmentRevision {
				return Invalid
			}
			active[v.NodeID] = true
		}
		ids[v.KeyID] = true
	}
	return nil
}
func (k ObserverKey) Validate() error {
	raw, e := base64.RawURLEncoding.DecodeString(k.Secret)
	if k.SchemaVersion != 1 || !ValidID(k.InstanceID) || !ValidID(k.NodeID) || !ValidID(k.KeyID) || !Decimal(k.Generation, false) || e != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != k.Secret {
		return Invalid
	}
	return nil
}
func (c Credential) Validate() error {
	if c.SchemaVersion != 1 {
		return Invalid
	}
	switch c.Kind {
	case "password":
		if c.Token != "" || c.AccessKeyID != "" || c.SecretAccessKey != "" || c.SessionToken != "" {
			return Invalid
		}
	case "bearer":
		if c.Password != "" || c.Token == "" || c.AccessKeyID != "" || c.SecretAccessKey != "" || c.SessionToken != "" {
			return Invalid
		}
	case "s3":
		if c.Password != "" || c.Token != "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
			return Invalid
		}
	default:
		return Invalid
	}
	return nil
}
func NewSecret() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic("secure randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func NewID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, strings.ToLower(rand.Text()[:16]))
}
