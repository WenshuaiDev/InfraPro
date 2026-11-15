// Package observer runs an independently authenticated node collector. Only the
// current snapshot and one exact unacknowledged body are kept in memory.
package observer

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/adapters"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/collector"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	runtimecfg "github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/runtime"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
)

type diskState struct {
	SchemaVersion int    `json:"schema_version"`
	InstanceID    string `json:"instance_id"`
	NodeID        string `json:"node_id"`
	Generation    string `json:"generation"`
	Epoch         string `json:"epoch"`
}
type Observer struct {
	config         settings.Observer
	configPath     string
	manifestDigest string
	bindingsDigest string
	manifest       settings.Manifest
	bindings       settings.Bindings
	key            settings.ObserverKey
	request        ingest.SessionRequest
	runner         *collector.Runner
	client         *http.Client
	lock           *securefile.Lock
	state          diskState
	token          string
	expires        time.Time
	sequence       uint64
	digest         string
	invalid        bool
}

func Open(path string) (*Observer, error) {
	var cfg settings.Observer
	if _, e := settings.Load(path, &cfg); e != nil || cfg.Validate() != nil {
		return nil, settings.Invalid
	}
	lock, e := securefile.Acquire(filepath.Join(cfg.StateDir, "observer.lock"))
	if e != nil {
		return nil, settings.Error("file_locked")
	}
	o := &Observer{config: cfg, configPath: path, lock: lock}
	if e = o.reload(true); e != nil {
		lock.Close()
		return nil, e
	}
	return o, nil
}
func (o *Observer) Close() {
	if o.client != nil {
		o.client.CloseIdleConnections()
	}
	_ = o.lock.Close()
}
func (o *Observer) reload(initial bool) error {
	var cfg settings.Observer
	configDigest, configErr := settings.Load(o.configPath, &cfg)
	if err := configErr; err != nil || cfg.Validate() != nil || cfg.InstanceID != o.config.InstanceID || cfg.NodeID != o.config.NodeID || cfg.StateDir != o.config.StateDir {
		return settings.Invalid
	}
	// Identity/state directory are fixed for the lifetime of the held lock.
	o.config = cfg
	var m settings.Manifest
	md, e := settings.Load(o.config.ManifestFile, &m)
	if e != nil || m.Validate(settings.DefaultLimits()) != nil || m.InstanceID != o.config.InstanceID {
		return settings.Invalid
	}
	var b settings.Bindings
	bd, e := settings.Load(o.config.BindingsFile, &b)
	if e != nil || b.Validate(m) != nil || b.NodeID != o.config.NodeID || b.AssignmentRevision != o.config.AssignmentRevision {
		return settings.Invalid
	}
	var key settings.ObserverKey
	kd, e := settings.Load(o.config.CredentialFile, &key)
	if e != nil || key.Validate() != nil || key.NodeID != o.config.NodeID || key.InstanceID != o.config.InstanceID {
		return settings.Invalid
	}
	caDigest := ""
	if cfg.TLSCAFile != "" {
		raw, err := securefile.Read(cfg.TLSCAFile, 1<<20, securefile.Policy{UID: os.Getuid(), Public: true})
		if err != nil {
			return settings.Invalid
		}
		caDigest = settings.Digest(raw)
	}
	digest := configDigest + caDigest + md + bd + kd + runtimecfg.CredentialDigest(b)
	if !initial && digest == o.digest && !o.invalid {
		return nil
	}
	if !initial && (settings.Number(m.Revision) < settings.Number(o.manifest.Revision) || m.Revision == o.manifest.Revision && md != o.manifestDigest || settings.Number(b.AssignmentRevision) < settings.Number(o.bindings.AssignmentRevision) || b.AssignmentRevision == o.bindings.AssignmentRevision && bd != o.bindingsDigest) {
		return settings.Error("config_rollback")
	}
	state := o.state
	if initial {
		path := filepath.Join(o.config.StateDir, "epoch.json")
		raw, err := securefile.Read(path, 4096, securefile.Private())
		if err == nil {
			if settings.Strict(raw, &state) != nil || state.SchemaVersion != 1 || state.InstanceID != o.config.InstanceID || state.NodeID != o.config.NodeID || !settings.Decimal(state.Epoch, false) || !settings.Decimal(state.Generation, false) {
				return settings.Invalid
			}
		} else if !os.IsNotExist(err) {
			return settings.Invalid
		}
	}
	if state.Generation != "" && settings.Number(key.Generation) < settings.Number(state.Generation) {
		return settings.Error("config_rollback")
	}
	changedGeneration := state.Generation != key.Generation
	if changedGeneration {
		state = diskState{1, o.config.InstanceID, o.config.NodeID, key.Generation, "0"}
	}
	if initial || changedGeneration {
		epoch, e := settings.Next(state.Epoch)
		if e != nil {
			return e
		}
		state.Epoch = epoch
		if e = settings.Save(filepath.Join(o.config.StateDir, "epoch.json"), state, false); e != nil {
			return settings.Invalid
		}
		o.state = state
		o.request.BootID = uuid.NewString()
		o.sequence = 0
	}
	client, e := clientFor(o.config)
	if e != nil {
		return e
	}
	if o.client != nil {
		o.client.CloseIdleConnections()
	}
	o.client = client
	o.request = ingest.SessionRequest{ProtocolMajor: 1, InstanceID: o.config.InstanceID, NodeID: o.config.NodeID, ManifestRevision: m.Revision, AssignmentRevision: b.AssignmentRevision, Epoch: state.Epoch, BootID: o.request.BootID}
	if o.runner == nil {
		o.runner = collector.New(o.request.BootID, 8)
	}
	// Changing generation starts a new collection identity, never reuses an old
	// success. On normal credential/topology reload the bounded runner is reused.
	if changedGeneration && !initial {
		o.runner.Boot(o.request.BootID)
	}
	o.runner.Update(m.Assigned(o.config.NodeID), b, adapters.New(m, b, o.request.BootID))
	o.manifest, o.bindings, o.key = m, b, key
	o.digest = digest
	o.manifestDigest = md
	o.bindingsDigest = bd
	o.token = ""
	o.invalid = false
	return nil
}
func clientFor(cfg settings.Observer) (*http.Client, error) {
	u, e := url.Parse(cfg.ServerURL)
	if e != nil {
		return nil, settings.Invalid
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	if cfg.TLSCAFile != "" {
		raw, e := securefile.Read(cfg.TLSCAFile, 1<<20, securefile.Policy{UID: os.Getuid(), Public: true})
		if e != nil {
			return nil, settings.Invalid
		}
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(raw) {
			return nil, settings.Invalid
		}
		tlsCfg.RootCAs = pool
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil || host != u.Hostname() {
			return nil, settings.Invalid
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil || len(ips) == 0 {
			return nil, settings.Error("dns_failed")
		}
		for _, ip := range ips {
			if !settings.SafeIP(ip.IP, true) {
				return nil, settings.Error("permission_denied")
			}
		}
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: dial, TLSClientConfig: tlsCfg, MaxConnsPerHost: 1, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
func (o *Observer) post(ctx context.Context, path, token string, raw []byte, out any) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, o.config.ServerURL+path, bytes.NewReader(raw))
	if e != nil {
		return settings.Invalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	response, e := o.client.Do(req)
	if e != nil {
		return settings.Error("connection_timeout")
	}
	defer response.Body.Close()
	body, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if e != nil || len(body) > 1<<20 {
		return settings.Error("invalid_report")
	}
	if response.StatusCode != 200 {
		var fail struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &fail)
		if settings.KnownReason(fail.Error.Code) {
			return settings.Error(fail.Error.Code)
		}
		return settings.Error("adapter_error")
	}
	if json.Unmarshal(body, out) != nil {
		return settings.Error("invalid_report")
	}
	return nil
}
func (o *Observer) handshake(ctx context.Context) error {
	raw, _ := json.Marshal(o.request)
	var session ingest.SessionResponse
	if e := o.post(ctx, "/observer/v1/sessions", o.key.KeyID+"."+o.key.Secret, raw, &session); e != nil {
		return e
	}
	if session.Token == "" || !settings.Decimal(session.LastAcceptedSequence, true) || session.MinReportIntervalMS < 5000 {
		return settings.Error("invalid_report")
	}
	o.token = session.Token
	o.expires = session.ExpiresAt
	o.sequence = max(o.sequence, settings.Number(session.LastAcceptedSequence))
	return nil
}
func (o *Observer) Run(ctx context.Context) error {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	go o.runner.Run(work)
	timer := time.NewTimer(0)
	defer timer.Stop()
	reload := time.NewTicker(5 * time.Second)
	defer reload.Stop()
	failures := 0
	var pending []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-reload.C:
			before := o.digest
			if e := o.reload(false); e != nil {
				o.invalid = true
				o.runner.Pause()
				o.token = ""
				pending = nil
			} else if before != o.digest {
				pending = nil
				timer.Reset(0)
			}
		case <-timer.C:
			if o.invalid {
				timer.Reset(5 * time.Second)
				continue
			}
			var e error
			if o.token == "" || time.Until(o.expires) < time.Minute {
				e = o.handshake(work)
			}
			if e == nil {
				// After a network interruption send a current full snapshot. Exact-byte
				// retry is used once for a potentially lost acknowledgement.
				if pending == nil {
					o.sequence++
					frame := ingest.Frame{SessionRequest: o.request, Sequence: strconv.FormatUint(o.sequence, 10), GeneratedAt: time.Now().UTC().Truncate(time.Millisecond), Observations: o.runner.Snapshot()}
					pending = encodeFrame(frame, o.manifest)
				}
				var ack ingest.Ack
				e = o.post(work, "/observer/v1/reports", o.token, pending, &ack)
				if e == nil {
					pending = nil
				}
			}
			delay := 5 * time.Second
			if e != nil {
				failures++
				code := settings.Code(e)
				delay = min(30*time.Second, time.Duration(1<<min(failures-1, 5))*time.Second)
				if failures > 1 {
					pending = nil
				}
				if settings.OneOf(code, "observer_session_expired", "observer_scope_denied") {
					o.token = ""
				}
				if settings.OneOf(code, "observer_scope_denied", "epoch_conflict", "manifest_mismatch", "assignment_mismatch", "stale_sequence", "sequence_conflict") {
					delay = 30 * time.Second
					pending = nil
				}
				delay = time.Duration(float64(delay) * (0.9 + rand.Float64()*0.2))
			} else {
				failures = 0
			}
			timer.Reset(delay)
		}
	}
}

// A complete assignment always fits the wire budget. Oversized evidence is an
// explicit failed observation, never a partial frame or an unbounded retry body.
func encodeFrame(frame ingest.Frame, manifest settings.Manifest) []byte {
	raw, err := json.Marshal(frame)
	if err == nil && len(raw) <= 1<<20 {
		return raw
	}
	frame.Observations = []model.Observation{}
	for _, p := range manifest.Probes {
		if p.NodeID != frame.NodeID {
			continue
		}
		o := model.Failure(model.New(p, frame.BootID, frame.GeneratedAt), "invalid_report", "response_limit_exceeded")
		o.Facets = model.UnknownFacets()
		frame.Observations = append(frame.Observations, o)
	}
	raw, _ = json.Marshal(frame)
	return raw
}
