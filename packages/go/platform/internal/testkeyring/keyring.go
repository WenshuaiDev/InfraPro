// Package testkeyring creates per-fixture keys outside the checkout. It is used
// only by test support; production identity never creates a default key.
package testkeyring

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
)

func Create() (string, func(), error) {
	dir, e := os.MkdirTemp("", "infrapro-mfa-test-")
	if e != nil {
		return "", func() {}, e
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		cleanup()
		return "", func() {}, e
	}
	body, _ := json.Marshal(map[string]any{"format_version": 1, "active_key_id": "fixture", "keys": []map[string]string{{"id": "fixture", "key_base64": base64.StdEncoding.EncodeToString(raw)}}})
	clear(raw)
	path := filepath.Join(dir, "keyring.json")
	if e = os.WriteFile(path, body, 0600); e != nil {
		cleanup()
		return "", func() {}, e
	}
	return path, cleanup, nil
}
