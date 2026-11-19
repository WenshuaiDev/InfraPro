package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/requestlimit"
)

// Architecture is a per-environment transport manifest. Each process receives
// its own certificate/key via the role-specific secret mount.
type Architecture struct {
	objectRequests *requestlimit.Limit
	slotIndex      int
	ReplicaSlots   map[string][]string `json:"replica_slots,omitempty"`
	ReplicaID      string              `json:"replica_id,omitempty"`
	ListenAddress  string              `json:"listen_address,omitempty"`
	Endpoints      map[string]string   `json:"endpoints"`
	CertDirectory  string              `json:"cert_directory"`
	Overrides      map[string]string   `json:"route_targets,omitempty"`
	Pools          map[string]int32    `json:"pg_connections,omitempty"`
	MaxInflight    map[string]int      `json:"max_inflight,omitempty"`
	RequestTimeout map[string]string   `json:"request_timeout,omitempty"`
	DisplaySplit   bool                `json:"split_display"`
	Role           string              `json:"-"`
}

const (
	Gateway  = "api"
	Identity = "identity-api"
	Control  = "control-api"
	Query    = "monitoring-query-api"
	Transfer = "transfer-api"
)

var Owners = []string{Identity, Control, Query, Transfer}

func readArchitecture(path, role string) (*Architecture, error) {
	if path == "" {
		return nil, nil
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var a Architecture
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if e = d.Decode(&a); e != nil {
		return nil, e
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("unexpected data in architecture file")
	}
	a.Role = role
	if a.CertDirectory == "" {
		return nil, errors.New("cert_directory required")
	}
	if (slices.Contains(Owners, role) || role == "compat-api" || role == "monitoring-incremental-worker") && a.Endpoints[role] == "" {
		return nil, fmt.Errorf("missing private listener for %s", role)
	}
	if !filepath.IsAbs(a.CertDirectory) {
		a.CertDirectory = filepath.Join(filepath.Dir(path), a.CertDirectory)
	}
	for name, addr := range a.Endpoints {
		u, e := url.Parse(addr)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("invalid private endpoint for %s", name)
		}
		if _, _, e = net.SplitHostPort(u.Host); e != nil {
			return nil, fmt.Errorf("missing private port for %s", name)
		}
	}
	for _, name := range Owners {
		if a.Endpoints[name] == "" {
			return nil, fmt.Errorf("missing private endpoint for %s", name)
		}
	}
	for name, n := range a.Pools {
		if n < 1 || n > 256 {
			return nil, fmt.Errorf("invalid pool for %s", name)
		}
	}
	for name, n := range a.MaxInflight {
		if n < 1 || n > 16384 {
			return nil, fmt.Errorf("invalid inflight limit for %s", name)
		}
	}
	for role, value := range a.RequestTimeout {
		d, e := time.ParseDuration(value)
		if !slices.Contains(Owners, role) || e != nil || d < time.Second || d > 24*time.Hour {
			return nil, fmt.Errorf("invalid request timeout for %s", role)
		}
	}
	for owner, target := range a.Overrides {
		if !slices.Contains(Owners, owner) || target != "compat-api" || a.Endpoints[target] == "" {
			return nil, fmt.Errorf("invalid route target for %s", owner)
		}
	}
	if err := a.selectSlot(); err != nil {
		return nil, err
	}
	a.objectRequests = requestlimit.New(a.Inflight(role))
	return &a, nil
}

var replicaName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func (a *Architecture) selectSlot() error {
	if id := os.Getenv("INFRAPRO_REPLICA_ID"); id != "" {
		if a.ReplicaID != "" && a.ReplicaID != id {
			return errors.New("configured and environment replica identities disagree")
		}
		a.ReplicaID = id
	}
	if addr := os.Getenv("INFRAPRO_PRIVATE_LISTEN_ADDR"); addr != "" {
		a.ListenAddress = addr
	}
	if a.ListenAddress != "" {
		_, port, err := net.SplitHostPort(a.ListenAddress)
		n, parseErr := strconv.ParseUint(port, 10, 16)
		if err != nil || parseErr != nil || n == 0 {
			return errors.New("invalid private replica listener")
		}
	}
	allSlots := make(map[string]bool)
	for role, ids := range a.ReplicaSlots {
		if !replicaName.MatchString(role) || len(ids) == 0 || len(ids) > 128 {
			return errors.New("invalid replica slot list")
		}
		for _, id := range ids {
			if !replicaName.MatchString(id) || allSlots[id] || len(a.ReplicaSlots[id]) > 0 || a.Endpoints[id] != "" || slices.Contains(Owners, id) || slices.Contains([]string{Gateway, "compat-api", "display-runtime", "diagnostics-observer", "monitoring-incremental-worker"}, id) {
				return errors.New("invalid or duplicate replica slot")
			}
			allSlots[id] = true
		}
		n := a.Pools[role]
		if n == 0 && role != Gateway && role != "parser" && role != "files-checker" {
			n = 12
		}
		if n > 0 && int(n) < len(ids) {
			return fmt.Errorf("PG quota below replica count for %s", role)
		}
		if a.inflightTotal(role) < len(ids) {
			return fmt.Errorf("inflight quota below replica count for %s", role)
		}
	}
	for _, owner := range Owners {
		if len(a.ReplicaSlots[Gateway]) > a.inflightTotal(owner) {
			return fmt.Errorf("outbound %s quota below gateway replica count", owner)
		}
	}
	slots := a.ReplicaSlots[a.Role]
	if len(slots) == 0 {
		if a.ReplicaID != "" && a.ReplicaID != a.Role+"-1" {
			return errors.New("replica is absent from role slot list")
		}
		a.ReplicaID = a.Role + "-1"
		return nil
	}
	if a.ReplicaID == "" && len(slots) == 1 {
		a.ReplicaID = slots[0]
	}
	a.slotIndex = slices.Index(slots, a.ReplicaID)
	if a.slotIndex < 0 {
		return errors.New("replica is absent from role slot list")
	}
	return nil
}

func (a *Architecture) Listener(role string) string {
	if a.Role == role && a.ListenAddress != "" {
		return a.ListenAddress
	}
	return a.Address(role)
}

func (a *Architecture) SlotKey() string {
	if a == nil || a.Role == "" || a.ReplicaID == "" {
		return ""
	}
	return a.Role + "/" + a.ReplicaID
}

// Share divides a role total between its configured process slots. The earliest
// slots receive one extra unit when the total is not divisible by their count.
func (a *Architecture) Share(total int) int {
	if a == nil || len(a.ReplicaSlots[a.Role]) < 2 {
		return total
	}
	n := len(a.ReplicaSlots[a.Role])
	share := total / n
	if a.slotIndex < total%n {
		share++
	}
	return share
}

func (a *Architecture) Address(role string) string {
	return strings.TrimPrefix(a.Endpoints[role], "https://")
}
func (a *Architecture) Target(owner string) string {
	if v := a.Overrides[owner]; v != "" {
		return v
	}
	return owner
}
func (a *Architecture) Connections(role string) int32 {
	if n := a.Pools[role]; n > 0 {
		return int32(a.Share(int(n)))
	}
	return int32(a.Share(12))
}
func (a *Architecture) inflightTotal(role string) int {
	if n := a.MaxInflight[role]; n > 0 {
		return n
	}
	return 32
}
func (a *Architecture) Inflight(role string) int { return a.Share(a.inflightTotal(role)) }

// A process serving private objects can use an S3-only deployment credential.
// Storage clients used by PG glossary/identity/gateway roles are never opened.
func roleMonitoring(raw input, role string, arch *Architecture) (Monitoring, error) {
	if arch == nil || role == "compat-api" || role == Control || role == Query || role == Transfer || strings.HasPrefix(role, "monitoring-") || (role == "display-runtime" && !arch.DisplaySplit) {
		return resolveMonitoring(raw)
	}
	object := Monitoring{}
	if role != Identity && role != Gateway {
		object.ObjectEndpoint = raw.MonitoringObjectEndpoint
		object.ObjectAccessKey = raw.MonitoringObjectAccessKey
		object.ObjectSecretKey = raw.MonitoringObjectSecretKey
		object.Bucket = raw.MonitoringBucket
		if object.ObjectEndpoint != "" || object.ObjectAccessKey != "" || object.ObjectSecretKey != "" || object.Bucket != "" {
			u, e := url.Parse(object.ObjectEndpoint)
			if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Host == "" || object.ObjectAccessKey == "" || object.ObjectSecretKey == "" || object.Bucket == "" {
				return object, errors.New("invalid private object storage configuration")
			}
		}
	}
	return object, nil
}

// Uses selects domain adapters for a split process. The compatible topology
// has the complete domain graph on both sides of an ownership switch.
func (a *Architecture) Uses(roles ...string) bool {
	return a == nil || a.Role == "compat-api" || slices.Contains(roles, a.Role)
}

func (a *Architecture) Timeout(role string) time.Duration {
	if value := a.RequestTimeout[role]; value != "" {
		if d, e := time.ParseDuration(value); e == nil {
			return d
		}
	}
	switch role {
	case Transfer:
		return 35 * time.Minute
	case Query:
		return 130 * time.Second
	default:
		return 60 * time.Second
	}
}

// ObjectRequests is shared by every private-object client in this role.
func (a *Architecture) ObjectRequests() *requestlimit.Limit {
	if a == nil {
		return nil
	}
	return a.objectRequests
}
