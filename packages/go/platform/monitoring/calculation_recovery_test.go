package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestCalculationBackupDescriptorRoundTripAndUnsupportedEngine(t *testing.T) {
	fixtures := json.RawMessage(`{"cases":[{"expected":"0.000000000000000000000001"}]}`)
	hash := sha256.Sum256(fixtures)
	current := CalculationRecoveryDescriptor{Layout: "1", Engine: "1", Compiler: "1", DSL: "1", Tables: []string{"derived_results_local"}, Fixtures: fixtures, FixtureSHA256: hex.EncodeToString(hash[:])}
	raw, e := json.MarshalIndent(BackupManifest{Version: 3, Calculations: &current}, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	var restored BackupManifest
	if e = json.Unmarshal(raw, &restored); e != nil {
		t.Fatal(e)
	}
	if !compatibleCalculationRecovery(*restored.Calculations, current) {
		t.Fatal("manifest formatting changed compatibility")
	}
	restored.Calculations.Engine = "unsupported"
	if compatibleCalculationRecovery(*restored.Calculations, current) {
		t.Fatal("unsupported engine accepted")
	}
	restored.Calculations.Engine = current.Engine
	restored.Calculations.Fixtures = json.RawMessage(`{"cases":[]}`)
	if compatibleCalculationRecovery(*restored.Calculations, current) {
		t.Fatal("fixture corruption accepted")
	}
}
