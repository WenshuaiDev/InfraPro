package parsingcontract

import (
	"encoding/base64"
	"os"
	"path/filepath"
)

// VerifyPackage checks original bytes and all retained executable identities. The
// required list comes from the owning database, never from a caller-selected subset.
func VerifyPackage(directory string, required []Entry) (Catalog, error) {
	var catalog Catalog
	raw, e := os.ReadFile(filepath.Join(directory, "catalog.json"))
	if e != nil {
		return catalog, Fail("missing_catalog")
	}
	if len(raw) > MaxResponse || StrictJSON(raw, &catalog) != nil || ValidateCatalog(catalog) != nil {
		return catalog, Fail("invalid_catalog")
	}
	present := map[string]Entry{}
	for _, entry := range catalog.Entries {
		present[entry.MethodID+"@"+entry.MethodVersion] = entry
		path := filepath.Join(directory, "artifacts", entry.ArtifactDigest)
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0222 != 0 {
			return catalog, Fail("artifact_unavailable")
		}
		raw, e = os.ReadFile(path)
		if e != nil || Digest(raw) != entry.ArtifactDigest {
			return catalog, Fail("artifact_digest_mismatch")
		}
		for _, v := range []struct{ name, encoded, digest string }{{"manifest.json", entry.ManifestBase64, entry.ManifestDigest}, {"outputs.json", entry.OutputContractBase64, entry.OutputContractDigest}} {
			original, e := os.ReadFile(filepath.Join(directory, "manifests", entry.ManifestDigest, v.name))
			if e != nil {
				return catalog, Fail("original_contract_missing")
			}
			expected, e := base64.StdEncoding.Strict().DecodeString(v.encoded)
			if e != nil || Digest(original) != v.digest || string(original) != string(expected) {
				return catalog, Fail("original_contract_mismatch")
			}
		}
	}
	for _, entry := range required {
		available, ok := present[entry.MethodID+"@"+entry.MethodVersion]
		if !ok {
			return catalog, Fail("required_artifact_missing")
		}
		if available.ManifestDigest != entry.ManifestDigest || available.ArtifactDigest != entry.ArtifactDigest || available.OutputContractDigest != entry.OutputContractDigest {
			return catalog, Fail("method_version_conflict")
		}
	}
	return catalog, nil
}
