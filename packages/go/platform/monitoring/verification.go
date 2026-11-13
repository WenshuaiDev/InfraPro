package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
	"github.com/jackc/pgx/v5"
)

type VerificationFinding struct {
	Channel string `json:"channel_id,omitempty"`
	Object  string `json:"object_id,omitempty"`
	Code    string `json:"code"`
}
type VerificationReport struct {
	Calculations json.RawMessage `json:"calculations,omitempty"`
	checkpoints  []SummaryRepairPlan
	Tenant       string                `json:"tenant_id"`
	Started      time.Time             `json:"started_at"`
	Finished     time.Time             `json:"finished_at"`
	State        string                `json:"state"`
	Channels     string                `json:"channels"`
	Chunks       string                `json:"chunks"`
	Rows         string                `json:"rows"`
	Files        string                `json:"source_files"`
	Bytes        string                `json:"source_bytes"`
	Findings     []VerificationFinding `json:"findings"`
}

func (s *Service) verifyChannelSources(ctx context.Context, tenant, channel string) error {
	_, _, e := s.verifySources(ctx, tenant, channel)
	return e
}
func (s *Service) verifySources(ctx context.Context, tenant, channel string) (int64, int64, error) {
	var files, bytes int64
	cursor := ""
	type file struct {
		id, key, hash string
		size          int64
	}
	for {
		page := []file{}
		e := s.db(ctx, func(tx pgx.Tx) error {
			rows, e := tx.Query(ctx, `SELECT f.id::text,f.object_key,f.sha256,f.size_bytes FROM monitoring_source_files f WHERE f.tenant_id=$1 AND ($2='' OR EXISTS(SELECT 1 FROM monitoring_import_batches b WHERE b.tenant_id=f.tenant_id AND b.source_file_id=f.id AND b.semantic_snapshot->'semantics' ? $2)) AND ($3='' OR f.id::text>$3) ORDER BY f.id LIMIT 100`, tenant, channel, cursor)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var f file
				if e = rows.Scan(&f.id, &f.key, &f.hash, &f.size); e != nil {
					return e
				}
				page = append(page, f)
			}
			return rows.Err()
		})
		if e != nil {
			return files, bytes, e
		}
		if len(page) == 0 {
			return files, bytes, nil
		}
		for _, f := range page {
			o, e := s.storage.get(ctx, f.key)
			if e != nil {
				return files, bytes, e
			}
			h := sha256.New()
			n, e := io.Copy(h, io.LimitReader(o, f.size+1))
			o.Close()
			if e != nil {
				return files, bytes, e
			}
			if n != f.size || hex.EncodeToString(h.Sum(nil)) != f.hash {
				return files, bytes, fmt.Errorf("source file checksum mismatch")
			}
			files++
			bytes += n
		}
		cursor = page[len(page)-1].id
	}
}
func (s *Service) contributionObject(ctx context.Context, tenant, chunk string) (string, error) {
	var key string
	e := s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE((SELECT r.object_key FROM monitoring_contribution_replacements r WHERE r.tenant_id=c.tenant_id AND r.chunk_id=c.id ORDER BY r.created_at DESC,r.job_id DESC LIMIT 1),c.summary_object) FROM monitoring_chunks c WHERE c.tenant_id=$1 AND c.id=$2`, tenant, chunk).Scan(&key)
	})
	return key, e
}
func (s *Service) verifyCanonicalChunk(ctx context.Context, tenant string, m selectedChunk) error {
	var key string
	e := s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT source_object FROM monitoring_chunks WHERE tenant_id=$1 AND id=$2`, tenant, m.Chunk).Scan(&key)
	})
	if e != nil {
		return e
	}
	o, e := s.storage.get(ctx, key)
	if e != nil {
		return e
	}
	it := rowsFrom(o)
	defer it.close()
	h := sha256.New()
	var n uint64
	for it.next() {
		io.WriteString(h, it.row.hash())
		n++
	}
	if it.err != nil {
		return it.err
	}
	if n != m.Count || hex.EncodeToString(h.Sum(nil)) != m.Hash {
		return fmt.Errorf("canonical chunk evidence mismatch")
	}
	return nil
}
func validateTotals(v summaryTotals) error {
	for _, group := range []map[string]exactState{v.Active, v.History} {
		for _, s := range group {
			for _, n := range []string{s.Sum, s.Squares} {
				if n != "" {
					if _, ok := new(big.Int).SetString(n, 10); !ok {
						return fmt.Errorf("invalid exact summary coefficient")
					}
				}
			}
			for _, p := range []*summaryPoint{s.First, s.Last, s.Min, s.Max} {
				if p != nil && p.Kind != "enum" {
					if _, e := observation.Coefficient(p.Value, 12); e != nil {
						return e
					}
				}
			}
		}
	}
	return nil
}
func (s *Service) verifySummaryTree(ctx context.Context, p SummaryRepairPlan, ref summaryRef, depth int, block int64) error {
	if ref.Object == "" {
		return nil
	}
	var node summaryNode
	if e := s.storage.summaryObject(ctx, ref.Object, &node, true); e != nil {
		return e
	}
	if e := validateTotals(node.Total); e != nil {
		return e
	}
	if node.Version != 1 || digest(node.Total) != digest(ref.Total) {
		return fmt.Errorf("summary node total mismatch")
	}
	sum := summaryTotals{}
	if depth > 0 {
		if len(node.Inputs) != 0 || len(node.Minutes) != 0 {
			return fmt.Errorf("branch contains leaf state")
		}
		for digit, child := range node.Children {
			d, e := strconv.ParseInt(digit, 10, 64)
			if e != nil || d < 0 || d > 255 {
				return fmt.Errorf("invalid summary radix")
			}
			if e = s.verifySummaryTree(ctx, p, child, depth-1, (block<<8)|d); e != nil {
				return e
			}
			sum.merge(child.Total)
		}
	} else {
		if len(node.Children) != 0 {
			return fmt.Errorf("leaf has children")
		}
		minutes := map[string]summaryTotals{}
		for chunk, v := range node.Inputs {
			var active bool
			e := s.db(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT p.revoke_sequence IS NULL OR p.revoke_sequence>$3 FROM monitoring_publication_chunks pc JOIN monitoring_publications p ON p.tenant_id=pc.tenant_id AND p.id=pc.publication_id WHERE pc.tenant_id=$1 AND pc.chunk_id=$2 AND p.sequence<=$3`, p.Tenant, chunk, p.Watermark).Scan(&active)
			})
			if e != nil {
				return e
			}
			if active != v.Active {
				return fmt.Errorf("summary adoption status mismatch")
			}
			key, e := s.contributionObject(ctx, p.Tenant, chunk)
			if e != nil {
				return e
			}
			var contribution chunkContributions
			if e = s.storage.summaryObject(ctx, key, &contribution, true); e != nil {
				return e
			}
			if digest(contribution[p.Channel][strconv.FormatInt(block, 10)]) != digest(v.Minutes) {
				return fmt.Errorf("summary leaf input differs from raw contribution")
			}
			for minute, total := range v.Minutes {
				if e = validateTotals(total); e != nil {
					return e
				}
				if !active {
					total = inactiveContribution(total)
				}
				current := minutes[minute]
				current.merge(total)
				minutes[minute] = current
			}
		}
		if digest(minutes) != digest(node.Minutes) {
			return fmt.Errorf("summary minute distribution mismatch")
		}
		for _, v := range minutes {
			sum.merge(v)
		}
	}
	if digest(sum) != digest(node.Total) {
		return fmt.Errorf("summary child aggregation mismatch")
	}
	return nil
}
func (s *Service) verifyChannel(ctx context.Context, p SummaryRepairPlan) (int64, int64, []VerificationFinding, error) {
	identity, e := newIdentityVerifier(s.storage.config.TempDirectory)
	if e != nil {
		return 0, 0, nil, e
	}
	defer identity.close()
	var chunks, rows int64
	findings := []VerificationFinding{}
	cursor := ""
	total := summaryTotals{}
	fail := func(id, code string) {
		if len(findings) < 100 {
			findings = append(findings, VerificationFinding{p.Channel, id, code})
		}
	}
	for {
		page, e := s.maintenancePage(ctx, p, cursor)
		if e != nil {
			return chunks, rows, findings, e
		}
		if len(page) == 0 {
			break
		}
		for _, m := range page {
			if e = s.storage.sealCheck(ctx, m); e != nil {
				fail(m.Chunk, "replica_payload_mismatch")
				continue
			}
			raw, e := s.storage.chunkRows(ctx, m)
			if e != nil {
				fail(m.Chunk, "raw_payload_mismatch")
				continue
			}
			if e = s.verifyCanonicalChunk(ctx, p.Tenant, m); e != nil {
				fail(m.Chunk, "canonical_evidence_mismatch")
			}
			var first uint64
			quality := map[string]string{"good": "0", "unmarked": "0", "suspect": "0", "bad": "0"}
			for _, r := range raw {
				if r.First {
					first++
				}
				n, _ := strconv.ParseUint(quality[r.Quality], 10, 64)
				quality[r.Quality] = strconv.FormatUint(n+1, 10)
				if r.Channel == p.Channel {
					total.add(r, !m.Revoked)
					if e = identity.add(r, !m.Revoked); e != nil {
						return chunks, rows, findings, e
					}
				}
			}
			if first != m.First || digest(quality) != digest(m.Qualities) {
				fail(m.Chunk, "adoption_count_mismatch")
			}
			key, e := s.contributionObject(ctx, p.Tenant, m.Chunk)
			if e != nil {
				return chunks, rows, findings, e
			}
			var contribution chunkContributions
			if e = s.storage.summaryObject(ctx, key, &contribution, true); e != nil || digest(contribution) != digest(makeContributions(raw)) {
				fail(m.Chunk, "contribution_mismatch")
			}
			chunks++
			rows += int64(len(raw))
		}
		cursor = page[len(page)-1].Chunk
	}
	if e = identity.verify(ctx, fail); e != nil {
		return chunks, rows, findings, e
	}
	root, e := s.readSummaryRoot(ctx, p.Tenant, p.Channel, p.Content)
	if e != nil {
		fail(p.Channel, "summary_missing_or_corrupt")
	} else if totalsDigest(total) != totalsDigest(root.Root.Total) {
		fail(p.Channel, "summary_exact_state_mismatch")
	} else if e = s.verifySummaryTree(ctx, p, root.Root, 3, 0); e != nil {
		fail(p.Channel, "summary_tree_mismatch")
	}
	current, e := s.PlanSummaryRepair(ctx, p.Tenant, p.Channel)
	if e != nil {
		return chunks, rows, findings, e
	}
	if current.Hash != p.Hash {
		return chunks, rows, findings, problem("verification_changed", "对账期间内容或路由发生变化，请重新核对。", 409)
	}
	return chunks, rows, findings, nil
}

// Verify is read-only. It never clears dirty ranges, repairs objects, deletes
// attempts, or changes quarantine. ApplyVerification is the separate write step.
func (s *Service) Verify(ctx context.Context, tenant string) (VerificationReport, error) {
	report := VerificationReport{Tenant: tenant, Started: time.Now().UTC(), State: "passed", Findings: []VerificationFinding{}}
	if !access.ValidID(tenant) {
		return report, fmt.Errorf("tenant is required")
	}
	if e := s.requireStorage(); e != nil {
		return report, e
	}
	var exists bool
	if e := s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1)`, tenant).Scan(&exists)
	}); e != nil {
		return report, e
	}
	if !exists {
		return report, notFound()
	}
	var watermark, route string
	e := s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE(s.commit_sequence,0)::text,COALESCE(s.route_revision,1)::text FROM tenants t LEFT JOIN monitoring_tenant_states s ON s.tenant_id=t.id WHERE t.id=$1`, tenant).Scan(&watermark, &route)
	})
	if e != nil {
		return report, e
	}
	var files, bytes, chunks, rows, count int64
	files, bytes, e = s.verifySources(ctx, tenant, "")
	if e != nil {
		report.Findings = append(report.Findings, VerificationFinding{Code: "source_evidence_unavailable"})
	}
	cursor := ""
	for {
		channels := []string{}
		e = s.db(ctx, func(tx pgx.Tx) error {
			rs, e := tx.Query(ctx, `SELECT channel_id::text FROM monitoring_channel_versions WHERE tenant_id=$1 AND content_revision>0 AND ($2='' OR channel_id::text>$2) ORDER BY channel_id LIMIT 100`, tenant, cursor)
			if e != nil {
				return e
			}
			defer rs.Close()
			for rs.Next() {
				var id string
				if e = rs.Scan(&id); e != nil {
					return e
				}
				channels = append(channels, id)
			}
			return rs.Err()
		})
		if e != nil {
			return report, e
		}
		if len(channels) == 0 {
			break
		}
		for _, id := range channels {
			p, e := s.PlanSummaryRepair(ctx, tenant, id)
			if e != nil {
				return report, e
			}
			report.checkpoints = append(report.checkpoints, p)
			n, c, found, e := s.verifyChannel(ctx, p)
			if e != nil {
				return report, e
			}
			chunks += n
			rows += c
			count++
			report.Findings = append(report.Findings, found...)
			if e = s.verifyChannelSources(ctx, tenant, id); e != nil {
				report.Findings = append(report.Findings, VerificationFinding{Channel: id, Code: "source_evidence_unavailable"})
			}
		}
		cursor = channels[len(channels)-1]
	}
	// Count logical published chunks once: a wide chunk is checked for each of
	// its channels, but those scans do not create extra observations. Reject a
	// concurrent tenant publication/revoke/route change across the whole run.
	e = s.db(ctx, func(tx pgx.Tx) error {
		var current, currentRoute string
		if e := tx.QueryRow(ctx, `SELECT COALESCE(s.commit_sequence,0)::text,COALESCE(s.route_revision,1)::text FROM tenants t LEFT JOIN monitoring_tenant_states s ON s.tenant_id=t.id WHERE t.id=$1`, tenant).Scan(&current, &currentRoute); e != nil {
			return e
		}
		if current != watermark || currentRoute != route {
			return problem("verification_changed", "对账期间发布或路由发生变化，请重新核对。", 409)
		}
		return tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(c.row_count),0)::bigint FROM monitoring_publication_chunks pc JOIN monitoring_chunks c ON c.tenant_id=pc.tenant_id AND c.id=pc.chunk_id JOIN monitoring_publications p ON p.tenant_id=pc.tenant_id AND p.id=pc.publication_id WHERE pc.tenant_id=$1 AND p.sequence<=$2`, tenant, watermark).Scan(&chunks, &rows)
	})
	if e != nil {
		return report, e
	}
	if len(report.Findings) > 0 {
		report.State = "failed"
	}
	report.Channels = countText(count)
	report.Chunks = countText(chunks)
	report.Rows = countText(rows)
	report.Files = countText(files)
	report.Bytes = countText(bytes)
	report.Finished = time.Now().UTC()
	return report, nil
}

func validateSummaryObject(value any) error {
	check := func(v summaryTotals) error { return validateTotals(v) }
	switch v := value.(type) {
	case *summaryRoot:
		return check(v.Root.Total)
	case *summaryNode:
		if e := check(v.Total); e != nil {
			return e
		}
		for _, r := range v.Children {
			if e := check(r.Total); e != nil {
				return e
			}
		}
		for _, m := range v.Minutes {
			if e := check(m); e != nil {
				return e
			}
		}
		for _, input := range v.Inputs {
			for _, m := range input.Minutes {
				if e := check(m); e != nil {
					return e
				}
			}
		}
	case *chunkContributions:
		for _, channel := range *v {
			for _, block := range channel {
				for _, m := range block {
					if e := check(m); e != nil {
						return e
					}
				}
			}
		}
	}
	return nil
}

func totalsDigest(v summaryTotals) string {
	if len(v.Active) == 0 {
		v.Active = nil
	}
	if len(v.History) == 0 {
		v.History = nil
	}
	return digest(v)
}
