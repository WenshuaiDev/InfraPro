package monitoring

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// BackupConfig is distinct from application source and raw-disk credentials.
// NamedCollection refers to a server-side secret reference, never a SQL secret.
type BackupConfig struct{ Endpoint, AccessKey, SecretKey, Bucket, NamedCollection string }
type BackupOptions struct {
	Directory, DatabaseURL, PGContainer, Reason string
	Store                                       BackupConfig
}
type ArchiveFile struct {
	Kind   string `json:"kind"`
	Key    string `json:"key,omitempty"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,string"`
}
type NativeBackup struct {
	Physical  string `json:"physical_shard"`
	Database  string `json:"database"`
	Prefix    string `json:"prefix"`
	Operation string `json:"operation_id"`
}
type BackupManifest struct {
	Version         int            `json:"version"`
	ID              string         `json:"id"`
	State           string         `json:"state"`
	Schema          string         `json:"control_schema"`
	Started         time.Time      `json:"started_at"`
	Completed       time.Time      `json:"completed_at"`
	PostgresVersion string         `json:"postgres_version"`
	LSN             string         `json:"checkpoint_lsn"`
	IndexSHA        string         `json:"file_index_sha256"`
	Files           string         `json:"files"`
	Bytes           string         `json:"bytes"`
	Native          []NativeBackup `json:"native_backups"`
	Tenants         []string       `json:"tenant_ids"`
	Reason          string         `json:"reason"`
}
type checkpointAttempt struct {
	Tenant string
	Entry  selectedChunk
}
type tenantCheckpoint struct{ Tenant, Watermark, Route string }

func backupClient(c BackupConfig) (*minio.Client, error) {
	u, e := url.Parse(c.Endpoint)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") || c.AccessKey == "" || c.SecretKey == "" || !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(c.Bucket) || !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(c.NamedCollection) {
		return nil, fmt.Errorf("invalid independent backup configuration")
	}
	return minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Secure: u.Scheme == "https", BucketLookup: minio.BucketLookupPath})
}
func pgCommand(ctx context.Context, dsn, container, tool string, args ...string) (*exec.Cmd, error) {
	c, e := pgx.ParseConfig(dsn)
	if e != nil {
		return nil, fmt.Errorf("invalid control database configuration")
	}
	env := map[string]string{"PGHOST": c.Host, "PGPORT": strconv.Itoa(int(c.Port)), "PGUSER": c.User, "PGPASSWORD": c.Password, "PGDATABASE": c.Database, "PGAPPNAME": "infrapro-monitoring-maintenance", "PGCONNECT_TIMEOUT": "10"}
	env["PGSSLMODE"] = "disable"
	if c.TLSConfig != nil {
		env["PGSSLMODE"] = "verify-full"
		if c.TLSConfig.InsecureSkipVerify {
			env["PGSSLMODE"] = "require"
			if len(c.Fallbacks) > 0 {
				env["PGSSLMODE"] = "prefer"
			}
		}
	}
	if u, e := url.Parse(dsn); e == nil {
		for _, key := range []string{"sslmode", "sslrootcert", "sslcert", "sslkey"} {
			if v := u.Query().Get(key); v != "" {
				env["PG"+strings.ToUpper(key)] = v
			}
		}
	}
	command := tool
	argv := args
	if container != "" {
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`).MatchString(container) {
			return nil, fmt.Errorf("invalid PostgreSQL tools container")
		}
		if c.Host == "127.0.0.1" || c.Host == "localhost" || c.Host == "::1" {
			env["PGHOST"] = "host.docker.internal"
		}
		command = "docker"
		argv = []string{"exec", "-i"}
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			argv = append(argv, "-e", k)
		}
		argv = append(argv, container, tool)
		argv = append(argv, args...)
	}
	cmd := exec.CommandContext(ctx, command, argv...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Tool stderr may include a credential-bearing connection description. Keep
	// public errors concise; the operation receipt identifies the failed phase.
	cmd.Stderr = io.Discard
	return cmd, nil
}
func archiveWrite(dir, path string, r io.Reader) (ArchiveFile, error) {
	out := ArchiveFile{Path: path}
	full := filepath.Join(dir, path)
	if e := os.MkdirAll(filepath.Dir(full), 0700); e != nil {
		return out, e
	}
	f, e := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return out, e
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), r)
	if e == nil {
		e = f.Sync()
	}
	closed := f.Close()
	if e == nil {
		e = closed
	}
	out.Size = n
	out.SHA256 = hex.EncodeToString(h.Sum(nil))
	return out, e
}
func archivePath(kind, key string) string {
	v := sha256.Sum256([]byte(key))
	return kind + "/" + hex.EncodeToString(v[:]) + ".blob"
}
func hashLocal(path string) (string, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, e
}
func receipt(dir, phase string, manifest BackupManifest) error {
	raw := encodeJSON(map[string]any{"phase": phase, "id": manifest.ID, "started_at": manifest.Started, "updated_at": time.Now().UTC()})
	temp := filepath.Join(dir, "receipt.json.next")
	if e := os.WriteFile(temp, raw, 0600); e != nil {
		return e
	}
	return os.Rename(temp, filepath.Join(dir, "receipt.json"))
}
func (s *Storage) physicalNames() []string {
	names := []string{"local"}
	for k := range s.shards {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
func (s *Storage) nativeOperation(ctx context.Context, sql, collection, prefix string) (string, error) {
	if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(collection) || !regexp.MustCompile(`^[a-z0-9/_-]+$`).MatchString(prefix) {
		return "", fmt.Errorf("invalid native backup reference")
	}
	id := uuid.NewString()
	settings := ""
	if strings.HasPrefix(sql, "RESTORE ") {
		settings = "allow_different_table_def=1,"
	}
	query := sql + " S3(" + collection + ",?) SETTINGS " + settings + "id=? ASYNC"
	var got string
	var state string
	cctx := clickhouse.Context(ctx, clickhouse.WithQueryID(id))
	if e := s.replicas[0].QueryRow(cctx, query, prefix, id).Scan(&got, &state); e != nil {
		return id, fmt.Errorf("native backup or restore could not start")
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		if state == "BACKUP_CREATED" || state == "RESTORED" {
			return id, nil
		}
		if strings.Contains(state, "FAILED") || state == "CANCELLED" {
			return id, fmt.Errorf("native backup or restore failed")
		}
		select {
		case <-ctx.Done():
			return id, ctx.Err()
		case <-ticker.C:
		}
		if e := s.replicas[0].QueryRow(ctx, `SELECT status FROM system.backups WHERE id=?`, id).Scan(&state); e != nil {
			return id, fmt.Errorf("native backup operation status unavailable")
		}
	}
}

// Backup creates a full application checkpoint. pg_dump imports the same
// exported repeatable-read snapshot used for all control manifests. Native CH
// parts may include later writes, but only the checkpoint is claimed restored.
func Backup(ctx context.Context, pool *pgxpool.Pool, storage *Storage, opt BackupOptions) (result BackupManifest, failure error) {
	m := BackupManifest{Version: 2, ID: uuid.NewString(), State: "incomplete", Started: time.Now().UTC(), Reason: opt.Reason, Native: []NativeBackup{}, Tenants: []string{}}
	if e := access.ValidReason(opt.Reason); e != nil {
		return m, e
	}
	if storage == nil {
		return m, fmt.Errorf("monitoring storage is required")
	}
	if e := storage.Check(ctx); e != nil {
		return m, e
	}
	client, e := backupClient(opt.Store)
	if e != nil {
		return m, e
	}
	if opt.Store.Endpoint == storage.config.ObjectEndpoint && opt.Store.Bucket == storage.config.Bucket {
		return m, fmt.Errorf("backup namespace must be independent")
	}
	exists, e := client.BucketExists(ctx, opt.Store.Bucket)
	if e != nil || !exists {
		return m, fmt.Errorf("independent backup bucket is unavailable")
	}
	if !filepath.IsAbs(opt.Directory) {
		return m, fmt.Errorf("backup directory must be absolute")
	}
	if e = os.Mkdir(opt.Directory, 0700); e != nil {
		return m, fmt.Errorf("backup requires a new directory")
	}
	defer func() {
		if failure != nil {
			_ = receipt(opt.Directory, "failed", result)
		}
	}()
	_ = receipt(opt.Directory, "checkpoint", m)
	index, e := os.OpenFile(filepath.Join(opt.Directory, "files.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return m, e
	}
	defer index.Close()
	enc := json.NewEncoder(index)
	var files, bytes int64
	add := func(v ArchiveFile) error { files++; bytes += v.Size; return enc.Encode(v) }
	tx, e := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return m, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var snapshot string
	if e = tx.QueryRow(ctx, `SELECT pg_export_snapshot(),current_schema(),current_setting('server_version'),pg_current_wal_lsn()::text`).Scan(&snapshot, &m.Schema, &m.PostgresVersion, &m.LSN); e != nil {
		return m, e
	}
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`).MatchString(m.Schema) {
		return m, fmt.Errorf("unsupported application control schema")
	}
	dump, e := os.OpenFile(filepath.Join(opt.Directory, "control.dump"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return m, e
	}
	cmd, e := pgCommand(ctx, opt.DatabaseURL, opt.PGContainer, "pg_dump", "--format=custom", "--no-owner", "--no-acl", "--schema="+m.Schema, "--exclude-table-data="+m.Schema+".monitoring_environment_state", "--snapshot="+snapshot)
	if e != nil {
		dump.Close()
		return m, e
	}
	cmd.Stdout = dump
	err := cmd.Run()
	if err == nil {
		err = dump.Sync()
	}
	dump.Close()
	if err != nil {
		return m, fmt.Errorf("control snapshot dump failed")
	}
	hash, n, e := hashLocal(filepath.Join(opt.Directory, "control.dump"))
	if e != nil {
		return m, e
	}
	if e = add(ArchiveFile{Kind: "control", Path: "control.dump", SHA256: hash, Size: n}); e != nil {
		return m, e
	}
	checkpoint, e := os.OpenFile(filepath.Join(opt.Directory, "checkpoint.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return m, e
	}
	writer := json.NewEncoder(checkpoint)
	rows, e := tx.Query(ctx, `SELECT tenant_id::text,commit_sequence::text,route_revision::text FROM monitoring_tenant_states ORDER BY tenant_id`)
	if e != nil {
		checkpoint.Close()
		return m, e
	}
	for rows.Next() {
		var c tenantCheckpoint
		if e = rows.Scan(&c.Tenant, &c.Watermark, &c.Route); e != nil {
			break
		}
		if e = writer.Encode(c); e != nil {
			break
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e == nil {
		e = checkpoint.Sync()
	}
	checkpoint.Close()
	if e != nil {
		return m, e
	}
	hash, n, e = hashLocal(filepath.Join(opt.Directory, "checkpoint.ndjson"))
	if e != nil {
		return m, e
	}
	if e = add(ArchiveFile{Kind: "checkpoint", Path: "checkpoint.ndjson", SHA256: hash, Size: n}); e != nil {
		return m, e
	}
	attempts, e := os.OpenFile(filepath.Join(opt.Directory, "attempts.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return m, e
	}
	writer = json.NewEncoder(attempts)
	rows, e = tx.Query(ctx, `SELECT DISTINCT c.tenant_id::text,c.id::text,a.id::text,a.physical_shard,c.payload_hash,c.row_count FROM monitoring_chunks c JOIN (SELECT tenant_id,chunk_id,attempt_id FROM monitoring_publication_chunks UNION SELECT tenant_id,chunk_id,attempt_id FROM monitoring_chunk_locations) pc ON pc.tenant_id=c.tenant_id AND pc.chunk_id=c.id JOIN monitoring_write_attempts a ON a.tenant_id=pc.tenant_id AND a.id=pc.attempt_id ORDER BY 1,2,3`)
	if e != nil {
		attempts.Close()
		return m, e
	}
	for rows.Next() {
		var v checkpointAttempt
		if e = rows.Scan(&v.Tenant, &v.Entry.Chunk, &v.Entry.Attempt, &v.Entry.Physical, &v.Entry.Hash, &v.Entry.Count); e != nil {
			break
		}
		if e = storage.sealCheck(ctx, v.Entry); e != nil {
			break
		}
		if e = writer.Encode(v); e != nil {
			break
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e == nil {
		e = attempts.Sync()
	}
	attempts.Close()
	if e != nil {
		return m, e
	}
	hash, n, e = hashLocal(filepath.Join(opt.Directory, "attempts.ndjson"))
	if e != nil {
		return m, e
	}
	if e = add(ArchiveFile{Kind: "attempts", Path: "attempts.ndjson", SHA256: hash, Size: n}); e != nil {
		return m, e
	}
	// Record all tenant prefixes at this control snapshot, including sources of
	// invalid/no-op batches which have no publication or channel-version row.
	rows, e = tx.Query(ctx, `SELECT id::text FROM tenants ORDER BY id`)
	if e != nil {
		return m, e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		m.Tenants = append(m.Tenants, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return m, e
	}
	_ = receipt(opt.Directory, "native-parts", m)
	for _, name := range storage.physicalNames() {
		p, _ := storage.physical(name)
		prefix := m.ID + "/clickhouse/" + name
		id, e := p.nativeOperation(ctx, "BACKUP DATABASE "+p.config.Database+" TO", opt.Store.NamedCollection, prefix)
		if e != nil {
			return m, e
		}
		m.Native = append(m.Native, NativeBackup{name, p.config.Database, prefix, id})
	}
	copyObjects := func(client *minio.Client, bucket, prefix, kind string) error {
		for v := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if v.Err != nil {
				return v.Err
			}
			o, e := client.GetObject(ctx, bucket, v.Key, minio.GetObjectOptions{})
			if e != nil {
				return e
			}
			file, e := archiveWrite(opt.Directory, archivePath(kind, v.Key), io.LimitReader(o, v.Size+1))
			o.Close()
			if e != nil {
				return e
			}
			if file.Size != v.Size {
				return fmt.Errorf("object length changed during backup")
			}
			file.Kind = kind
			file.Key = v.Key
			if e = add(file); e != nil {
				return e
			}
		}
		return ctx.Err()
	}
	_ = receipt(opt.Directory, "objects", m)
	for _, id := range m.Tenants {
		if e = copyObjects(storage.objects, storage.config.Bucket, id+"/", "source"); e != nil {
			return m, e
		}
	}
	for _, b := range m.Native {
		if e = copyObjects(client, opt.Store.Bucket, b.Prefix+"/", "native"); e != nil {
			return m, e
		}
	}
	// Required PG references must be present even if an object listing was
	// incomplete. The immutable tree graph is rechecked by the actual restore.
	refs, e := tx.Query(ctx, `SELECT object_key FROM monitoring_source_files UNION SELECT object_key FROM monitoring_upload_parts UNION SELECT source_object FROM monitoring_chunks UNION SELECT summary_object FROM monitoring_chunks UNION SELECT validation_object FROM monitoring_import_batches WHERE validation_object IS NOT NULL UNION SELECT report_object FROM monitoring_import_batches WHERE report_object IS NOT NULL UNION SELECT object_key FROM monitoring_batch_resolutions UNION SELECT root_object FROM monitoring_summary_heads UNION SELECT root_object FROM monitoring_summary_replacements UNION SELECT object_key FROM monitoring_contribution_replacements UNION SELECT manifest_object FROM monitoring_exports WHERE manifest_object IS NOT NULL UNION SELECT result->>'object' FROM monitoring_job_steps WHERE result ? 'object'`)
	if e != nil {
		return m, e
	}
	for refs.Next() {
		var key string
		if e = refs.Scan(&key); e != nil {
			break
		}
		if key != "" {
			if _, e = os.Stat(filepath.Join(opt.Directory, archivePath("source", key))); e != nil {
				break
			}
		}
	}
	if e == nil {
		e = refs.Err()
	}
	refs.Close()
	if e != nil {
		return m, fmt.Errorf("checkpoint references an object absent from backup")
	}
	filesToCheck, e := tx.Query(ctx, `SELECT object_key,sha256,size_bytes FROM monitoring_source_files`)
	if e != nil {
		return m, e
	}
	for filesToCheck.Next() {
		var key, expected string
		var size int64
		if e = filesToCheck.Scan(&key, &expected, &size); e != nil {
			break
		}
		var actual string
		var n int64
		actual, n, e = hashLocal(filepath.Join(opt.Directory, archivePath("source", key)))
		if e != nil || actual != expected || n != size {
			e = fmt.Errorf("backup source file differs from its sealed checksum")
			break
		}
	}
	if e == nil {
		e = filesToCheck.Err()
	}
	filesToCheck.Close()
	if e != nil {
		return m, e
	}
	if e = tx.Commit(ctx); e != nil {
		return m, e
	}
	if e = index.Sync(); e != nil {
		return m, e
	}
	if e = index.Close(); e != nil {
		return m, e
	}
	m.IndexSHA, _, e = hashLocal(filepath.Join(opt.Directory, "files.ndjson"))
	if e != nil {
		return m, e
	}
	m.State = "complete"
	m.Completed = time.Now().UTC()
	m.Files = countText(files)
	m.Bytes = countText(bytes)
	raw := encodeJSON(m)
	if _, e = archiveWrite(opt.Directory, "manifest.json", strings.NewReader(string(raw))); e != nil {
		return m, e
	}
	hash, _, e = hashLocal(filepath.Join(opt.Directory, "manifest.json"))
	if e != nil {
		return m, e
	}
	if _, e = archiveWrite(opt.Directory, "manifest.sha256", strings.NewReader(hash+"\n")); e != nil {
		return m, e
	}
	if e = receipt(opt.Directory, "complete", m); e != nil {
		return m, e
	}
	return m, nil
}

func ReadBackupManifest(dir, expectedHash string) (BackupManifest, error) {
	var m BackupManifest
	if !filepath.IsAbs(dir) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(expectedHash) {
		return m, fmt.Errorf("absolute archive directory and expected manifest SHA-256 are required")
	}
	hash, n, e := hashLocal(filepath.Join(dir, "manifest.json"))
	if e != nil || hash != expectedHash || n > 4<<20 {
		return m, fmt.Errorf("backup manifest checksum mismatch")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return m, e
	}
	if e = json.Unmarshal(raw, &m); e != nil {
		return m, e
	}
	if m.Version != 2 || m.State != "complete" || !access.ValidID(m.ID) || !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`).MatchString(m.Schema) {
		return m, fmt.Errorf("unsupported or incomplete backup")
	}
	hash, _, e = hashLocal(filepath.Join(dir, "files.ndjson"))
	if e != nil || hash != m.IndexSHA {
		return m, fmt.Errorf("backup file index checksum mismatch")
	}
	var count, bytes int64
	e = walkArchive(dir, func(f ArchiveFile) error {
		count++
		bytes += f.Size
		h, n, e := hashLocal(filepath.Join(dir, f.Path))
		if e != nil || n != f.Size || h != f.SHA256 {
			return fmt.Errorf("backup member checksum mismatch")
		}
		return nil
	})
	if e != nil {
		return m, e
	}
	if countText(count) != m.Files || countText(bytes) != m.Bytes {
		return m, fmt.Errorf("backup index totals mismatch")
	}
	for _, b := range m.Native {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(b.Database) || b.Prefix != m.ID+"/clickhouse/"+b.Physical {
			return m, fmt.Errorf("invalid native backup descriptor")
		}
	}
	return m, nil
}
func walkArchive(dir string, consume func(ArchiveFile) error) error {
	f, e := os.Open(filepath.Join(dir, "files.ndjson"))
	if e != nil {
		return e
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 65536), 1<<20)
	for scanner.Scan() {
		var v ArchiveFile
		if e = json.Unmarshal(scanner.Bytes(), &v); e != nil {
			return e
		}
		if v.Size < 0 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(v.SHA256) {
			return fmt.Errorf("invalid archive member")
		}
		switch v.Kind {
		case "source", "native":
			if v.Key == "" || strings.HasPrefix(v.Key, "/") || strings.Contains(v.Key, "..") || v.Path != archivePath(v.Kind, v.Key) {
				return fmt.Errorf("invalid object archive path")
			}
		case "control":
			if v.Path != "control.dump" {
				return fmt.Errorf("invalid control archive path")
			}
		case "checkpoint":
			if v.Path != "checkpoint.ndjson" {
				return fmt.Errorf("invalid checkpoint archive path")
			}
		case "attempts":
			if v.Path != "attempts.ndjson" {
				return fmt.Errorf("invalid attempts archive path")
			}
		default:
			return fmt.Errorf("unknown backup member kind")
		}
		info, e := os.Lstat(filepath.Join(dir, v.Path))
		if e != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("backup member must be a regular file")
		}
		if e = consume(v); e != nil {
			return e
		}
	}
	return scanner.Err()
}
func Restore(ctx context.Context, pool *pgxpool.Pool, storage *Storage, opt BackupOptions, expectedHash string) (BackupManifest, error) {
	m, e := ReadBackupManifest(opt.Directory, expectedHash)
	if e != nil {
		return m, e
	}
	if e = access.ValidReason(opt.Reason); e != nil {
		return m, e
	}
	client, e := backupClient(opt.Store)
	if e != nil {
		return m, e
	}
	var tables int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')`).Scan(&tables); e != nil {
		return m, e
	}
	if tables != 0 {
		return m, fmt.Errorf("restore requires an empty independent control database")
	}
	if len(m.Native) != len(storage.physicalNames()) {
		return m, fmt.Errorf("restore physical shard mapping is incomplete")
	}
	for _, b := range m.Native {
		p, e := storage.physical(b.Physical)
		if e != nil {
			return m, e
		}
		for _, c := range p.replicas {
			var exists uint8
			if e = c.QueryRow(ctx, "EXISTS TABLE "+p.config.Database+".observations").Scan(&exists); e != nil {
				return m, e
			}
			if exists != 0 {
				return m, fmt.Errorf("restore requires new physical tables")
			}
		}
	}
	for o := range storage.objects.ListObjects(ctx, storage.config.Bucket, minio.ListObjectsOptions{Recursive: true, MaxKeys: 1}) {
		if o.Err != nil {
			return m, o.Err
		}
		return m, fmt.Errorf("restore source bucket must be empty")
	}
	for o := range client.ListObjects(ctx, opt.Store.Bucket, minio.ListObjectsOptions{Prefix: m.ID + "/", Recursive: true, MaxKeys: 1}) {
		if o.Err != nil {
			return m, o.Err
		}
		return m, fmt.Errorf("restore backup staging prefix must be empty")
	}
	// The destination is empty and offline. Once control tables exist, the guard
	// blocks all monitoring reads and user tasks until reconciliation succeeds.
	if _, e = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public`); e != nil {
		return m, e
	}
	dump, e := os.Open(filepath.Join(opt.Directory, "control.dump"))
	if e != nil {
		return m, e
	}
	cmd, e := pgCommand(ctx, opt.DatabaseURL, opt.PGContainer, "pg_restore", "--exit-on-error", "--no-owner", "--no-acl", "--single-transaction", "--dbname="+pool.Config().ConnConfig.Database)
	if e != nil {
		dump.Close()
		return m, e
	}
	cmd.Stdin = dump
	cmd.Stdout = io.Discard
	err := cmd.Run()
	dump.Close()
	if err != nil {
		return m, fmt.Errorf("control database restore failed")
	}
	if _, e = pool.Exec(ctx, `INSERT INTO monitoring_environment_state(id,mode,backup_id,verified_at,access_revoked) VALUES(1,'restoring',$1,NULL,false) ON CONFLICT(id) DO UPDATE SET mode='restoring',backup_id=EXCLUDED.backup_id,verified_at=NULL,access_revoked=false`, m.ID); e != nil {
		return m, fmt.Errorf("restored control schema does not match the configured search_path")
	}
	if e = identity.NewRepository(pool).RevokeAfterRestore(ctx, identity.Meta{Reason: opt.Reason, RequestID: uuid.NewString()}); e != nil {
		return m, e
	}
	if _, e = pool.Exec(ctx, `UPDATE monitoring_environment_state SET access_revoked=true WHERE id=1`); e != nil {
		return m, e
	}
	if _, e = pool.Exec(ctx, `UPDATE monitoring_snapshots SET expires_at=LEAST(expires_at,clock_timestamp()),lease_until=NULL; UPDATE monitoring_exports SET invalidated_at=COALESCE(invalidated_at,clock_timestamp())`); e != nil {
		return m, e
	}
	if e = storage.Initialize(ctx); e != nil {
		return m, e
	}
	e = walkArchive(opt.Directory, func(v ArchiveFile) error {
		var dest *minio.Client
		var bucket string
		switch v.Kind {
		case "source":
			dest, bucket = storage.objects, storage.config.Bucket
		case "native":
			dest, bucket = client, opt.Store.Bucket
		default:
			return nil
		}
		body, e := os.Open(filepath.Join(opt.Directory, v.Path))
		if e != nil {
			return e
		}
		defer body.Close()
		opts := minio.PutObjectOptions{SendContentMd5: true, PartSize: uint64(UploadPartSize), NumThreads: 1}
		opts.SetMatchETagExcept("*")
		if _, e = dest.PutObject(ctx, bucket, v.Key, body, v.Size, opts); e != nil {
			return e
		}
		o, e := dest.GetObject(ctx, bucket, v.Key, minio.GetObjectOptions{})
		if e != nil {
			return e
		}
		defer o.Close()
		h := sha256.New()
		n, e := io.Copy(h, io.LimitReader(o, v.Size+1))
		if e != nil {
			return e
		}
		if n != v.Size || hex.EncodeToString(h.Sum(nil)) != v.SHA256 {
			return fmt.Errorf("restored object readback mismatch")
		}
		return nil
	})
	if e != nil {
		return m, e
	}
	for _, b := range m.Native {
		p, _ := storage.physical(b.Physical)
		if _, e = p.nativeOperation(ctx, "RESTORE TABLE "+b.Database+".observations AS "+p.config.Database+".observations FROM", opt.Store.NamedCollection, b.Prefix); e != nil {
			return m, e
		}
		for _, c := range p.replicas {
			if e = c.Exec(ctx, "SYSTEM SYNC REPLICA "+p.config.Database+".observations"); e != nil {
				return m, e
			}
		}
	}
	if e = storage.Check(ctx); e != nil {
		return m, e
	}
	// Prove every original and retained selected attempt, not merely the visible
	// row total. Revoked publications and duplicate evidence remain in control.
	attempts, e := os.Open(filepath.Join(opt.Directory, "attempts.ndjson"))
	if e != nil {
		return m, e
	}
	decoder := json.NewDecoder(attempts)
	for {
		var v checkpointAttempt
		e = decoder.Decode(&v)
		if errors.Is(e, io.EOF) {
			e = nil
			break
		}
		if e != nil {
			break
		}
		if e = storage.sealCheck(ctx, v.Entry); e != nil {
			break
		}
	}
	attempts.Close()
	if e != nil {
		return m, e
	}
	service := New(access.New(pool, slog.Default())).WithStorage(storage)
	for _, tenant := range m.Tenants {
		report, e := service.Verify(ctx, tenant)
		if e != nil {
			return m, e
		}
		repair := map[string]bool{}
		for _, finding := range report.Findings {
			if finding.Channel != "" && (strings.HasPrefix(finding.Code, "summary_") || finding.Code == "contribution_mismatch") {
				repair[finding.Channel] = true
			} else {
				return m, fmt.Errorf("restored raw or source evidence failed reconciliation")
			}
		}
		for channel := range repair {
			p, e := service.PlanSummaryRepair(ctx, tenant, channel)
			if e != nil {
				return m, e
			}
			job, e := service.QueueSummaryRepair(ctx, p, "恢复后重建派生摘要："+opt.Reason)
			if e != nil {
				return m, e
			}
			for {
				state, e := service.InspectOperatorJob(ctx, tenant, job)
				if e != nil {
					return m, e
				}
				if state.State == "succeeded" {
					break
				}
				if state.State == "failed" || state.State == "needs_action" || state.State == "retry_wait" {
					return m, fmt.Errorf("restored summary rebuild failed")
				}
				worked, e := service.RunOne(ctx, "restore-"+m.ID)
				if e != nil {
					return m, e
				}
				if !worked {
					return m, fmt.Errorf("restore maintenance task unavailable")
				}
			}
		}
		report, e = service.Verify(ctx, tenant)
		if e != nil {
			return m, e
		}
		if report.State != "passed" {
			return m, fmt.Errorf("restored environment failed reconciliation")
		}
	}
	_, e = pool.Exec(ctx, `UPDATE monitoring_environment_state SET mode='read_only',verified_at=clock_timestamp() WHERE id=1 AND backup_id=$1 AND access_revoked`, m.ID)
	return m, e
}
func OpenRestoredWrites(ctx context.Context, pool *pgxpool.Pool, storage *Storage, backupID, reason string) error {
	if !access.ValidID(backupID) {
		return fmt.Errorf("backup ID is required")
	}
	if e := access.ValidReason(reason); e != nil {
		return e
	}
	if e := storage.Check(ctx); e != nil {
		return e
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var mode string
	var matched bool
	if e = tx.QueryRow(ctx, `SELECT mode,backup_id=$1 AND access_revoked AND verified_at IS NOT NULL FROM monitoring_environment_state WHERE id=1 FOR UPDATE`, backupID).Scan(&mode, &matched); e != nil {
		return e
	}
	if !matched || mode != "read_only" {
		return fmt.Errorf("restore is not verified and awaiting write release")
	}
	if _, e = tx.Exec(ctx, `UPDATE monitoring_environment_state SET mode='ready' WHERE id=1`); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO monitoring_audit(tenant_id,operation,target_id,reason,trace_id,summary) SELECT tenant_id,'operator.restore.open',$1::uuid,$2,$1::text,jsonb_build_object('backup_id',$1::text) FROM monitoring_tenant_states`, backupID, reason); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
