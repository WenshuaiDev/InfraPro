package ingestioncontract

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"os"
	"path/filepath"
	"slices"
)

// VerifyPackage checks exact immutable capability identities and every executable,
// alongside the existing measurement contract. It is used before both upgrades
// and rollbacks; an old executable cannot silently replace a registered version.
func VerifyPackage(directory string, measurements []c.Entry, required Catalog) (Catalog, error) {
	out := Catalog{}
	measurement, e := c.VerifyPackage(directory, measurements)
	if e != nil {
		return out, e
	}
	b, e := os.ReadFile(filepath.Join(directory, "ingestion-capabilities.json"))
	if e != nil {
		return out, c.Fail("ingestion_catalog_unavailable")
	}
	if e = c.StrictJSON(b, &out); e != nil {
		return out, e
	}
	if e = ValidateCatalog(out, measurement); e != nil {
		return out, e
	}
	for _, v := range required.IdentificationMethods {
		if !slices.Contains(out.IdentificationMethods, v) {
			return out, c.Fail("release_identification_missing")
		}
	}
	for _, v := range required.IngressProtocols {
		found := false
		for _, x := range out.IngressProtocols {
			if x.ID == v.ID && x.Version == v.Version && x.Digest == v.Digest {
				found = true
			}
		}
		if !found {
			return out, c.Fail("release_protocol_missing")
		}
	}
	for _, v := range required.Compatibilities {
		found := false
		for _, x := range out.Compatibilities {
			if x.ID == v.ID && x.Revision == v.Revision && x.Digest == v.Digest {
				found = true
			}
		}
		if !found {
			return out, c.Fail("release_compatibility_missing")
		}
	}
	for _, protocol := range out.IngressProtocols {
		a := protocol.Adapter
		supported := a == (Adapter{ID: ddp.AdapterID, Version: ddp.AdapterVersion, ArtifactDigest: c.Digest([]byte(ddp.AdapterSource))}) || a == (Adapter{ID: "https.raw-body", Version: "1.0.0", ArtifactDigest: c.Digest([]byte("https.raw-body/v1:mtls,4MiB,raw-body,source-scoped-idempotency"))})
		if !supported {
			return out, c.Fail("release_adapter_missing")
		}
	}
	for _, v := range out.IdentificationMethods {
		path := filepath.Join(directory, "artifacts", v.Method.ArtifactDigest)
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0222 != 0 || info.Mode().Perm()&0111 == 0 {
			return out, c.Fail("artifact_unavailable")
		}
		b, e := os.ReadFile(path)
		if e != nil || c.Digest(b) != v.Method.ArtifactDigest {
			return out, c.Fail("artifact_digest_mismatch")
		}
	}
	return out, nil
}
