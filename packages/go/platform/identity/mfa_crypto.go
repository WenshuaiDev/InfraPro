package identity

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
)

type mfaKeyring struct {
	active string
	keys   map[string]cipher.AEAD
	random io.Reader
}
type sealedSecret struct {
	Ciphertext, Nonce []byte
	KeyID             string
}

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func loadMFAKeyring(path string) (*mfaKeyring, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("mfa_keyring_file must be an absolute path")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, fmt.Errorf("MFA keyring unavailable")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return nil, fmt.Errorf("MFA keyring must be a private regular file (0600)")
	}
	var v struct {
		Format int    `json:"format_version"`
		Active string `json:"active_key_id"`
		Keys   []struct {
			ID  string `json:"id"`
			Key string `json:"key_base64"`
		} `json:"keys"`
	}
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if e = d.Decode(&v); e != nil {
		return nil, fmt.Errorf("invalid MFA keyring")
	}
	var extra any
	if d.Decode(&extra) != io.EOF || v.Format != 1 || len(v.Keys) == 0 || len(v.Keys) > 32 {
		return nil, fmt.Errorf("invalid MFA keyring format")
	}
	k := &mfaKeyring{active: v.Active, keys: map[string]cipher.AEAD{}, random: rand.Reader}
	for _, item := range v.Keys {
		raw, e := base64.StdEncoding.Strict().DecodeString(item.Key)
		if e != nil || len(raw) != 32 || !keyIDPattern.MatchString(item.ID) || k.keys[item.ID] != nil {
			return nil, fmt.Errorf("invalid MFA key entry")
		}
		block, e := aes.NewCipher(raw)
		clear(raw)
		if e != nil {
			return nil, e
		}
		a, e := cipher.NewGCM(block)
		if e != nil {
			return nil, e
		}
		k.keys[item.ID] = a
	}
	if k.keys[k.active] == nil {
		return nil, fmt.Errorf("MFA active key is missing")
	}
	return k, nil
}

func mfaAAD(account, purpose string, version int64) []byte {
	b, _ := json.Marshal([]string{"infrapro/mfa/v1", account, purpose, strconv.FormatInt(version, 10)})
	return b
}
func (k *mfaKeyring) seal(secret, account, purpose string, version int64) (sealedSecret, error) {
	a := k.keys[k.active]
	nonce := make([]byte, a.NonceSize())
	if _, e := io.ReadFull(k.random, nonce); e != nil {
		return sealedSecret{}, e
	}
	return sealedSecret{a.Seal(nil, nonce, []byte(secret), mfaAAD(account, purpose, version)), nonce, k.active}, nil
}
func (k *mfaKeyring) open(v sealedSecret, account, purpose string, version int64) (string, error) {
	a := k.keys[v.KeyID]
	if a == nil || len(v.Nonce) != a.NonceSize() {
		return "", ErrUnavailable
	}
	b, e := a.Open(nil, v.Nonce, v.Ciphertext, mfaAAD(account, purpose, version))
	if e != nil {
		return "", ErrUnavailable
	}
	defer clear(b)
	return string(b), nil
}
func (k *mfaKeyring) newSecret() (string, error) {
	b := make([]byte, 20)
	if _, e := io.ReadFull(k.random, b); e != nil {
		return "", e
	}
	defer clear(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// Return the greatest matching step. A consumed step (including enrollment)
// cannot be reused after process restarts, cross-node races or clock rollback.
func matchTOTPStep(secret, code string, now time.Time, last *int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	matched := int64(-1)
	for offset := int64(-1); offset <= 1; offset++ {
		step := now.Unix()/30 + offset
		if step < 0 {
			continue
		}
		expected, e := hotp.GenerateCodeCustom(secret, uint64(step), hotp.ValidateOpts{Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if e == nil && subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 && (last == nil || step > *last) {
			matched = step
		}
	}
	return matched, matched >= 0
}
func enrollmentQR(secret, issuer, label string) (string, error) {
	key, e := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: label, Period: 30, Secret: mustDecodeSecret(secret), Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if e != nil {
		return "", e
	}
	img, e := key.Image(256, 256)
	if e != nil {
		return "", e
	}
	var b bytes.Buffer
	if e = png.Encode(&b, img); e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}
func mustDecodeSecret(secret string) []byte {
	b, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	return b
}
func normalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}
func recoveryDigest(account string, generation int64, code string) string {
	b, _ := json.Marshal([]string{"infrapro/mfa/recovery/v1", account, strconv.FormatInt(generation, 10), normalizeRecoveryCode(code)})
	return digest(string(b))
}
func challengeDigest(token string) string { return digest("infrapro/mfa/challenge/v1:" + token) }
func grantDigest(token string) string     { return digest("infrapro/mfa/grant/v1:" + token) }
func (k *mfaKeyring) recoveryCodes() ([]string, error) {
	codes := make([]string, 10)
	for i := range codes {
		b := make([]byte, 16)
		if _, e := io.ReadFull(k.random, b); e != nil {
			return nil, e
		}
		raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
		clear(b)
		var groups []string
		for len(raw) > 4 {
			groups = append(groups, raw[:4])
			raw = raw[4:]
		}
		groups = append(groups, raw)
		codes[i] = strings.Join(groups, "-")
	}
	return codes, nil
}
