// parser-package creates a release without ever replacing a version's artifact.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/methods"
	"os"
	"path/filepath"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("usage: parser-package RUNNER SAMPLES_DIRECTORY OUTPUT_DIRECTORY")
	}
	b, e := os.ReadFile(os.Args[1])
	if e != nil {
		return e
	}
	digest := c.Digest(b)
	suite, e := os.ReadFile(filepath.Join(os.Args[2], "manifest.json"))
	if e != nil {
		return e
	}
	dir := os.Args[3]
	if e = os.MkdirAll(filepath.Join(dir, "artifacts"), 0755); e != nil {
		return e
	}
	catalog := c.Catalog{ContractVersion: c.Version, ServiceRelease: "package-1", ServiceBuildDigest: digest, Entries: []c.Entry{}}
	old, e := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if e == nil {
		if e = c.StrictJSON(old, &catalog); e != nil {
			return e
		}
		if e = c.ValidateCatalog(catalog); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	for _, m := range methods.Manifests(digest, c.Digest(suite)) {
		found := false
		for _, entry := range catalog.Entries {
			if entry.MethodID == m.MethodID && entry.MethodVersion == m.MethodVersion {
				found = true
				break
			}
		}
		if !found {
			catalog.Entries = append(catalog.Entries, methods.Entry(m))
		}
	}
	path := filepath.Join(dir, "artifacts", digest)
	if old, e := os.ReadFile(path); e == nil {
		if c.Digest(old) != digest {
			return c.Fail("artifact_digest_mismatch")
		}
	} else if os.IsNotExist(e) {
		if e = os.WriteFile(path, b, 0555); e != nil {
			return e
		}
	} else {
		return e
	}
	for _, entry := range catalog.Entries {
		old, e := os.ReadFile(filepath.Join(dir, "artifacts", entry.ArtifactDigest))
		if e != nil || c.Digest(old) != entry.ArtifactDigest {
			return c.Fail("missing_retained_artifact")
		}
	}
	for _, entry := range catalog.Entries {
		base := filepath.Join(dir, "manifests", entry.ManifestDigest)
		if e = os.MkdirAll(base, 0755); e != nil {
			return e
		}
		for name, encoded := range map[string]string{"manifest.json": entry.ManifestBase64, "outputs.json": entry.OutputContractBase64} {
			body, e := base64.StdEncoding.Strict().DecodeString(encoded)
			if e != nil {
				return e
			}
			path := filepath.Join(base, name)
			if old, e := os.ReadFile(path); e == nil {
				if string(old) != string(body) {
					return c.Fail("original_contract_mismatch")
				}
			} else if os.IsNotExist(e) {
				if e = os.WriteFile(path, body, 0444); e != nil {
					return e
				}
			} else {
				return e
			}
		}
	}
	raw, _ := json.MarshalIndent(catalog, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "catalog.json.new"), raw, 0444); e != nil {
		return e
	}
	return os.Rename(filepath.Join(dir, "catalog.json.new"), filepath.Join(dir, "catalog.json"))
}
