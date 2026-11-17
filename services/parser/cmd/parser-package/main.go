// parser-package creates a release without ever replacing a version's artifact.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/identification"
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
	ingestionSuite, e := os.ReadFile(filepath.Join(os.Args[2], "../ingestion/expected.json"))
	if e != nil {
		return e
	}
	capabilities := identification.Catalog(identification.Methods(digest, c.Digest(ingestionSuite)), catalog)
	if raw, err := os.ReadFile(filepath.Join(dir, "ingestion-capabilities.json")); err == nil {
		var old i.Catalog
		if err = c.StrictJSON(raw, &old); err != nil {
			return err
		}
		if err = i.ValidateCatalog(old, catalog); err != nil {
			return err
		}
		// Previously published identification artifacts and their manifests retain
		// their exact identity even when this packaging executable is updated.
		for n, method := range capabilities.IdentificationMethods {
			for _, retained := range old.IdentificationMethods {
				if method.Method.ID == retained.Method.ID && method.Method.Version == retained.Method.Version {
					capabilities.IdentificationMethods[n] = retained
				}
			}
		}
		capabilities = identification.Catalog(capabilities.IdentificationMethods, catalog)
	} else if !os.IsNotExist(err) {
		return err
	}
	if e = i.ValidateCatalog(capabilities, catalog); e != nil {
		return e
	}
	for _, method := range capabilities.IdentificationMethods {
		raw, err := os.ReadFile(filepath.Join(dir, "artifacts", method.Method.ArtifactDigest))
		if err != nil || c.Digest(raw) != method.Method.ArtifactDigest {
			return c.Fail("missing_retained_identification_artifact")
		}
	}
	capraw, _ := json.MarshalIndent(capabilities, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "ingestion-capabilities.json.new"), capraw, 0444); e != nil {
		return e
	}
	if e = os.Rename(filepath.Join(dir, "ingestion-capabilities.json.new"), filepath.Join(dir, "ingestion-capabilities.json")); e != nil {
		return e
	}
	raw, _ := json.MarshalIndent(catalog, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "catalog.json.new"), raw, 0444); e != nil {
		return e
	}
	return os.Rename(filepath.Join(dir, "catalog.json.new"), filepath.Join(dir, "catalog.json"))
}
