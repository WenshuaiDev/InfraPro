package compute

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
)

//go:embed recovery_fixtures.json
var recoveryFixtures []byte

// RecoveryFixtures is retained in every calculation backup. It contains no
// tenant data and is qualified against this exact engine in the fixture tests.
func RecoveryFixtures() (json.RawMessage, string) {
	var compact bytes.Buffer
	if e := json.Compact(&compact, recoveryFixtures); e != nil {
		panic(e)
	}
	data := compact.Bytes()
	hash := sha256.Sum256(data)
	return json.RawMessage(data), hex.EncodeToString(hash[:])
}
