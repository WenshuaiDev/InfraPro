// Package settings owns diagnostics' offline configuration contract. It never
// initializes business services, listeners or database connections.
package settings

import "encoding/json"

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	InstanceID    string       `json:"instance_id"`
	Revision      string       `json:"revision"`
	DisplayName   string       `json:"display_name"`
	Nodes         []Node       `json:"nodes"`
	Components    []Component  `json:"components"`
	Replicas      []Replica    `json:"replicas"`
	Endpoints     []Endpoint   `json:"endpoints"`
	Dependencies  []Dependency `json:"dependencies"`
	Probes        []Probe      `json:"probes"`
}
type Node struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"display_name"`
	Kind               string `json:"kind"`
	ParentNodeID       string `json:"parent_node_id,omitempty"`
	ResourceScope      string `json:"resource_scope"`
	AssignmentRevision string `json:"assignment_revision"`
	Enabled            bool   `json:"enabled"`
}
type Component struct {
	ID               string `json:"id"`
	DisplayName      string `json:"display_name"`
	Kind             string `json:"kind"`
	Expectation      string `json:"expectation"`
	MinReadyReplicas int    `json:"min_ready_replicas"`
	// A static explanation makes disabled/on-demand catalog entries actionable.
	ExpectationReason string `json:"expectation_reason,omitempty"`
}
type Replica struct {
	ID               string   `json:"id"`
	ComponentID      string   `json:"component_id"`
	NodeID           string   `json:"node_id"`
	Expectation      string   `json:"expectation"`
	RequiredProbeIDs []string `json:"required_probe_ids"`
}
type Endpoint struct {
	ID          string `json:"id"`
	ReplicaID   string `json:"replica_id"`
	Purpose     string `json:"purpose"`
	DisplayName string `json:"display_name"`
}
type Dependency struct {
	ID            string `json:"id"`
	FromReplicaID string `json:"from_replica_id"`
	ToComponentID string `json:"to_component_id"`
	ProbeID       string `json:"probe_id"`
	Required      bool   `json:"required"`
	EvidenceMode  string `json:"evidence_mode"`
}
type Probe struct {
	ID           string `json:"id"`
	NodeID       string `json:"node_id"`
	TargetKind   string `json:"target_kind"`
	TargetID     string `json:"target_id"`
	Adapter      string `json:"adapter"`
	BindingID    string `json:"binding_id"`
	Facet        string `json:"facet"`
	Required     bool   `json:"required"`
	IntervalMS   int    `json:"interval_ms"`
	TimeoutMS    int    `json:"timeout_ms"`
	StaleAfterMS int    `json:"stale_after_ms"`
}
type Server struct {
	SchemaVersion      int            `json:"schema_version"`
	InstanceID         string         `json:"instance_id"`
	Listen             string         `json:"listen"`
	PublicOrigin       string         `json:"public_origin"`
	AllowedHosts       []string       `json:"allowed_hosts"`
	TLS                *TLSFiles      `json:"tls"`
	TrustedProxyCIDRs  []string       `json:"trusted_proxy_cidrs"`
	ManifestFile       string         `json:"manifest_file"`
	AuthFile           string         `json:"auth_file"`
	NodeKeysFile       string         `json:"node_keys_file"`
	StateDir           string         `json:"state_dir"`
	LocalObserver      *LocalObserver `json:"local_observer"`
	Limits             Limits         `json:"limits"`
	ResourceHints      ResourceHints  `json:"resource_hints"`
	DevelopmentOrigins []string       `json:"development_origins,omitempty"`
}
type TLSFiles struct {
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}
type LocalObserver struct {
	NodeID             string `json:"node_id"`
	AssignmentRevision string `json:"assignment_revision"`
	BindingsFile       string `json:"bindings_file"`
}
type Limits struct {
	MaxNodes         int   `json:"max_nodes,omitempty"`
	MaxReplicas      int   `json:"max_replicas,omitempty"`
	MaxProbes        int   `json:"max_probes,omitempty"`
	ProbesPerNode    int   `json:"probes_per_node,omitempty"`
	ConcurrentProbes int   `json:"concurrent_probes,omitempty"`
	StoreMaxBytes    int64 `json:"store_max_bytes,omitempty"`
	HistoryPoints    int   `json:"history_points,omitempty"`
}
type ResourceHints struct {
	Enabled              bool    `json:"enabled"`
	CPURatio             float64 `json:"cpu_ratio"`
	CPUSamples           int     `json:"cpu_samples"`
	MemoryAvailableRatio float64 `json:"memory_available_ratio"`
	DiskAvailableRatio   float64 `json:"disk_available_ratio"`
	DiskAvailableBytes   string  `json:"disk_available_bytes"`
}
type Observer struct {
	SchemaVersion      int    `json:"schema_version"`
	InstanceID         string `json:"instance_id"`
	NodeID             string `json:"node_id"`
	AssignmentRevision string `json:"assignment_revision"`
	ServerURL          string `json:"server_url"`
	ManifestFile       string `json:"manifest_file"`
	BindingsFile       string `json:"bindings_file"`
	CredentialFile     string `json:"credential_file"`
	StateDir           string `json:"state_dir"`
	TLSCAFile          string `json:"tls_ca_file,omitempty"`
}
type Bindings struct {
	SchemaVersion      int       `json:"schema_version"`
	InstanceID         string    `json:"instance_id"`
	NodeID             string    `json:"node_id"`
	AssignmentRevision string    `json:"assignment_revision"`
	AllowedTargetHosts []string  `json:"allowed_target_hosts,omitempty"`
	Bindings           []Binding `json:"bindings"`
}

// Binding is a discriminated union, checked by UnmarshalJSON and Validate.
// No adapter has a command, arbitrary SQL or arbitrary request-header field.
type Binding struct {
	ID                 string                     `json:"id"`
	Adapter            string                     `json:"adapter"`
	ReportFile         string                     `json:"report_file,omitempty"`
	PIDFile            string                     `json:"pid_file,omitempty"`
	PIDFileFormat      string                     `json:"pid_file_format,omitempty"`
	AllowedRoot        string                     `json:"allowed_root,omitempty"`
	ExpectedExecutable string                     `json:"expected_executable,omitempty"`
	ExpectedUID        *int                       `json:"expected_uid,omitempty"`
	ExpectedReplicaID  string                     `json:"expected_replica_id,omitempty"`
	PIDNamespace       string                     `json:"pid_namespace,omitempty"`
	DependencyKey      string                     `json:"dependency_key,omitempty"`
	LaneID             string                     `json:"lane_id,omitempty"`
	Scope              string                     `json:"scope,omitempty"`
	MountPaths         []Mount                    `json:"mount_paths,omitempty"`
	NetworkInterfaces  []NetworkInterface         `json:"network_interfaces,omitempty"`
	UnitName           string                     `json:"unit_name,omitempty"`
	SocketPath         string                     `json:"socket_path,omitempty"`
	SelectorLabels     map[string]string          `json:"selector_labels,omitempty"`
	StatsEnabled       bool                       `json:"stats_enabled,omitempty"`
	URL                string                     `json:"url,omitempty"`
	ExpectedStatus     int                        `json:"expected_status,omitempty"`
	ExpectedJSON       map[string]json.RawMessage `json:"expected_json,omitempty"`
	CredentialFile     string                     `json:"credential_file,omitempty"`
	TLSCAFile          string                     `json:"tls_ca_file,omitempty"`
	Host               string                     `json:"host,omitempty"`
	Port               int                        `json:"port,omitempty"`
	TLS                bool                       `json:"tls,omitempty"`
	ServerName         string                     `json:"server_name,omitempty"`
	Database           string                     `json:"database,omitempty"`
	Username           string                     `json:"username,omitempty"`
	TLSMode            string                     `json:"tls_mode,omitempty"`
	Address            string                     `json:"address,omitempty"`
	Endpoint           string                     `json:"endpoint,omitempty"`
	Region             string                     `json:"region,omitempty"`
	Bucket             string                     `json:"bucket,omitempty"`
	Enabled            bool                       `json:"enabled,omitempty"`
}
type Mount struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}
type NetworkInterface struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Auth struct {
	SchemaVersion int    `json:"schema_version"`
	Revision      string `json:"revision"`
	Users         []User `json:"users"`
}
type User struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	PasswordHash string   `json:"password_hash"`
	Enabled      bool     `json:"enabled"`
	Permissions  []string `json:"permissions"`
}
type Keyring struct {
	SchemaVersion int       `json:"schema_version"`
	InstanceID    string    `json:"instance_id"`
	Revision      string    `json:"revision"`
	Keys          []NodeKey `json:"keys"`
}
type NodeKey struct {
	KeyID              string `json:"key_id"`
	NodeID             string `json:"node_id"`
	Generation         string `json:"generation"`
	SecretSHA256       string `json:"secret_sha256"`
	Enabled            bool   `json:"enabled"`
	AssignmentRevision string `json:"assignment_revision"`
}
type ObserverKey struct {
	SchemaVersion int    `json:"schema_version"`
	InstanceID    string `json:"instance_id"`
	NodeID        string `json:"node_id"`
	KeyID         string `json:"key_id"`
	Generation    string `json:"generation"`
	Secret        string `json:"secret"`
}
type Credential struct {
	SchemaVersion   int    `json:"schema_version"`
	Kind            string `json:"kind"`
	Password        string `json:"password,omitempty"`
	Token           string `json:"token,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	SessionToken    string `json:"session_token,omitempty"`
}
