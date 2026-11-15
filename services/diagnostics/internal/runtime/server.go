// Package runtime wires the independent server. Its dependency graph contains no
// business bootstrap, identity, platform runtime or business database pool.
package runtime

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/access"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/adapters"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/collector"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/store"
	httptransport "github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/transport/http"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/process"
)

type Server struct {
	Config                                                        settings.Server
	Access                                                        *access.Manager
	Engine                                                        *ingest.Engine
	Collector                                                     *collector.Runner
	HTTP                                                          *httptransport.Server
	db                                                            *store.Store
	localBoot                                                     string
	sequence                                                      uint64
	manifestDigest, keysDigest, bindingsDigest, credentialsDigest string
	hadState                                                      bool
	lastRecovery                                                  time.Time
	localManifest                                                 settings.Manifest
	localBindings                                                 settings.Bindings
}

func Open(path string, development bool) (*Server, error) {
	var cfg settings.Server
	if _, err := settings.Load(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.Validate(development) != nil {
		return nil, settings.Invalid
	}
	_ = cfg.Limits.Normalize()
	db, err := store.Open(cfg.StateDir, cfg.InstanceID, cfg.Limits)
	if err != nil && settings.OneOf(settings.Code(err), "file_locked", "state_instance_mismatch", "state_version_unsupported") {
		return nil, err
	}
	local := ""
	if cfg.LocalObserver != nil {
		local = cfg.LocalObserver.NodeID
	}
	engine, err := ingest.New(db, cfg.InstanceID, local)
	if err != nil {
		if db != nil {
			db.Close()
		}
		return nil, err
	}
	s := &Server{Config: cfg, Access: access.New(), Engine: engine, db: db, localBoot: uuid.NewString()}
	if local != "" {
		s.Collector = collector.New(s.localBoot, cfg.Limits.ConcurrentProbes)
	}
	_, stateErr := os.Lstat(filepath.Join(cfg.StateDir, "state.db"))
	s.hadState = stateErr == nil || db != nil
	s.HTTP = httptransport.New(cfg, s.Access, engine, s.Collector)
	s.Reload(context.Background())
	return s, nil
}
func (s *Server) Close() {
	if s.db != nil {
		_ = s.db.Close()
	}
}
func (s *Server) Reload(ctx context.Context) {
	if s.db != nil && time.Since(s.lastRecovery) >= 15*time.Second {
		s.lastRecovery = time.Now()
		_ = s.Engine.RetryStore(ctx)
	}
	if s.db == nil && time.Since(s.lastRecovery) >= 15*time.Second {
		s.lastRecovery = time.Now()
		_, exists := os.Lstat(filepath.Join(s.Config.StateDir, "state.db"))
		if !s.hadState || exists == nil {
			candidate, err := store.Open(s.Config.StateDir, s.Config.InstanceID, s.Config.Limits)
			if err == nil {
				if s.Engine.AttachStore(ctx, candidate) == nil {
					s.db = candidate
					s.hadState = true
				} else {
					_ = candidate.Close()
				}
			}
		}
	}

	var auth settings.Auth
	digest, err := settings.Load(s.Config.AuthFile, &auth)
	if err != nil {
		s.Access.Invalidate()
	} else {
		_ = s.Access.Reload(auth, digest)
	}
	var m settings.Manifest
	md, err := settings.Load(s.Config.ManifestFile, &m)
	if err != nil || m.Validate(s.Config.Limits) != nil || m.InstanceID != s.Config.InstanceID {
		s.Engine.ConfigFailed()
		return
	}
	if md != s.manifestDigest || s.Engine.Read().ConfigState != "ok" {
		if err = s.Engine.SetManifest(ctx, m, s.Config.Limits); err != nil {
			s.Engine.ConfigFailed()
			return
		}
		s.manifestDigest = md
	}
	var keys settings.Keyring
	kd, err := settings.Load(s.Config.NodeKeysFile, &keys)
	local := ""
	if s.Config.LocalObserver != nil {
		local = s.Config.LocalObserver.NodeID
	}
	if err != nil || keys.Validate(m, local) != nil {
		s.Engine.KeysFailed()
	} else if kd != s.keysDigest || s.Engine.Read().KeyringState != "ok" {
		if err = s.Engine.SetKeys(ctx, keys); err != nil {
			s.Engine.KeysFailed()
		} else {
			s.keysDigest = kd
		}
	}
	if s.Collector == nil {
		return
	}
	var bindings settings.Bindings
	bd, err := settings.Load(s.Config.LocalObserver.BindingsFile, &bindings)
	if err != nil || bindings.Validate(m) != nil || bindings.NodeID != local || bindings.AssignmentRevision != assignment(m, local) {
		s.Collector.Pause()
		s.Engine.ConfigFailed()
		return
	}
	for _, b := range bindings.Bindings {
		if b.Adapter == "docker_container" || b.Adapter == "self_report" && b.PIDNamespace == "container" {
			s.Collector.Pause()
			s.Engine.ConfigFailed()
			return
		}
	}
	cd := CredentialDigest(bindings)
	if bd != s.bindingsDigest || cd != s.credentialsDigest || s.localManifest.Revision != m.Revision {
		s.Collector.Update(m.Assigned(local), bindings, adapters.New(m, bindings, s.localBoot))
		s.bindingsDigest, s.credentialsDigest = bd, cd
		s.localManifest, s.localBindings = m, bindings
	}
}

// CredentialDigest includes missing/unreadable markers. A rotation invalidates
// cached success before the next collection; driver connections are one-shot.
func CredentialDigest(bindings settings.Bindings) string {
	data := []byte{}
	for _, b := range bindings.Bindings {
		if b.CredentialFile == "" {
			continue
		}
		raw, e := securefile.Read(b.CredentialFile, 4096, securefile.Private())
		if e != nil {
			data = append(data, []byte("unavailable")...)
		} else {
			data = append(data, []byte(settings.Digest(raw))...)
		}
	}
	return settings.Digest(data)
}
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.Config.Listen)
	if err != nil {
		return settings.Error("state_unavailable")
	}
	server := s.HTTP.HTTPServer()
	done := make(chan error, 1)
	if s.Config.TLS != nil {
		cert, e := securefile.Read(s.Config.TLS.CertFile, 1<<20, securefile.Policy{UID: os.Getuid(), Public: true})
		if e != nil {
			listener.Close()
			return settings.Invalid
		}
		key, e := securefile.Read(s.Config.TLS.KeyFile, 1<<20, securefile.Private())
		if e != nil {
			listener.Close()
			return settings.Invalid
		}
		if err = installTLS(server, cert, key); err != nil {
			listener.Close()
			return err
		}
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.Collector != nil {
		go s.Collector.Run(work)
	}
	s.writeIdentity()
	go func() {
		if s.Config.TLS != nil {
			done <- server.ServeTLS(listener, "", "")
		} else {
			done <- server.Serve(listener)
		}
	}()
	slog.Info("diagnostics server listening", "origin", s.Config.PublicOrigin)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	s.localFrame(work)
	for {
		select {
		case err := <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return settings.Error("adapter_error")
		case <-ctx.Done():
			cancel()
			shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			err = server.Shutdown(shutdown)
			if err != nil {
				_ = server.Close()
			}
			return nil
		case <-ticker.C:
			s.Reload(work)
			s.localFrame(work)
			s.Engine.Tick(work)
		}
	}
}
func assignment(m settings.Manifest, node string) string {
	n, _ := m.Node(node)
	return n.AssignmentRevision
}
func (s *Server) localFrame(ctx context.Context) {
	if s.Collector == nil || s.localManifest.Revision == "" {
		return
	}
	s.sequence++
	node := s.Config.LocalObserver.NodeID
	request := ingest.SessionRequest{ProtocolMajor: 1, InstanceID: s.Config.InstanceID, NodeID: node, ManifestRevision: s.localManifest.Revision, AssignmentRevision: s.localBindings.AssignmentRevision, Epoch: "1", BootID: s.localBoot}
	_ = s.Engine.AcceptLocal(ctx, ingest.Frame{SessionRequest: request, Sequence: strconv.FormatUint(s.sequence, 10), GeneratedAt: time.Now().UTC().Truncate(time.Millisecond), Observations: s.Collector.Snapshot()})
}
func (s *Server) writeIdentity() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	proc, e := process.NewProcessWithContext(ctx, int32(os.Getpid()))
	if e != nil {
		return
	}
	created, e := proc.CreateTimeWithContext(ctx)
	if e != nil {
		return
	}
	_ = settings.Save(filepath.Join(s.Config.StateDir, "process.json"), struct {
		PID   int    `json:"pid"`
		Start string `json:"os_start_time_ms"`
	}{os.Getpid(), strconv.FormatInt(created, 10)}, false)
}
