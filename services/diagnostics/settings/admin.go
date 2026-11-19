package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
)

func LoadServer(path string) (Server, error) {
	var s Server
	if _, e := Load(path, &s); e != nil {
		return s, e
	}
	if e := s.Validate(false); e != nil {
		return s, e
	}
	_ = s.Limits.Normalize()
	return s, nil
}
func ValidateFiles(path string) error {
	s, e := LoadServer(path)
	if e != nil {
		return e
	}
	var m Manifest
	if _, e = Load(s.ManifestFile, &m); e != nil || m.Validate(s.Limits) != nil || m.InstanceID != s.InstanceID {
		return Invalid
	}
	var a Auth
	if _, e = Load(s.AuthFile, &a); e != nil || a.Validate() != nil {
		return Invalid
	}
	var k Keyring
	local := ""
	if s.LocalObserver != nil {
		local = s.LocalObserver.NodeID
	}
	if _, e = Load(s.NodeKeysFile, &k); e != nil || k.Validate(m, local) != nil {
		return Invalid
	}
	if s.LocalObserver != nil {
		var b Bindings
		if _, e = Load(s.LocalObserver.BindingsFile, &b); e != nil || b.Validate(m) != nil || b.NodeID != local || b.AssignmentRevision != s.LocalObserver.AssignmentRevision {
			return Invalid
		}
		for _, v := range b.Bindings {
			if v.Adapter == "docker_container" || v.Adapter == "self_report" && v.PIDNamespace == "container" {
				return Invalid
			}
		}
	}
	return securefile.CheckDirectory(s.StateDir, securefile.Private())
}
func EditAuth(serverFile, action, username, password string, enabled *bool) error {
	s, e := LoadServer(serverFile)
	if e != nil {
		return e
	}
	username = strings.ToLower(username)
	if !usernamePattern.MatchString(username) {
		return Invalid
	}
	lock, e := securefile.Acquire(filepath.Join(filepath.Dir(serverFile), "config-edit.lock"))
	if e != nil {
		return Error("file_locked")
	}
	defer lock.Close()
	var auth Auth
	raw, e := securefile.Read(s.AuthFile, 1<<20, securefile.Private())
	if e != nil {
		if !errors.Is(e, os.ErrNotExist) || action != "init" {
			return Invalid
		}
		auth = Auth{SchemaVersion: 1, Revision: "0", Users: []User{}}
	} else if Strict(raw, &auth) != nil || auth.Validate() != nil {
		return Invalid
	}
	found := -1
	for i, u := range auth.Users {
		if u.Username == username {
			found = i
		}
	}
	switch action {
	case "init", "add":
		if found >= 0 || len(auth.Users) >= 32 || action == "init" && len(auth.Users) > 0 {
			return Invalid
		}
		hash, e := PasswordHash(password)
		if e != nil {
			return e
		}
		auth.Users = append(auth.Users, User{ID: NewID("user"), Username: username, PasswordHash: hash, Enabled: true, Permissions: []string{"diagnostics.read"}})
	case "reset-password":
		if found < 0 {
			return Invalid
		}
		hash, e := PasswordHash(password)
		if e != nil {
			return e
		}
		auth.Users[found].PasswordHash = hash
	case "set-enabled":
		if found < 0 || enabled == nil {
			return Invalid
		}
		auth.Users[found].Enabled = *enabled
	default:
		return Invalid
	}
	auth.Revision, e = Next(auth.Revision)
	if e != nil {
		return e
	}
	if auth.Validate() != nil {
		return Invalid
	}
	return Save(s.AuthFile, auth, false)
}
func EditNodeKey(serverFile, action, node, output string) error {
	s, e := LoadServer(serverFile)
	if e != nil {
		return e
	}
	lock, e := securefile.Acquire(filepath.Join(filepath.Dir(serverFile), "config-edit.lock"))
	if e != nil {
		return Error("file_locked")
	}
	defer lock.Close()
	var m Manifest
	if _, e = Load(s.ManifestFile, &m); e != nil || m.Validate(s.Limits) != nil {
		return Invalid
	}
	n, ok := m.Node(node)
	if !ok || !n.Enabled || s.LocalObserver != nil && node == s.LocalObserver.NodeID {
		return Invalid
	}
	var keys Keyring
	local := ""
	if s.LocalObserver != nil {
		local = s.LocalObserver.NodeID
	}
	if _, e = Load(s.NodeKeysFile, &keys); e != nil || keys.Validate(m, local) != nil {
		return Invalid
	}
	generation := uint64(0)
	active := false
	for _, k := range keys.Keys {
		if k.NodeID == node {
			generation = max(generation, Number(k.Generation))
			active = active || k.Enabled
		}
	}
	if action == "issue" && active || !OneOf(action, "issue", "rotate", "revoke") || generation == ^uint64(0) {
		return Invalid
	}
	for i := range keys.Keys {
		if keys.Keys[i].NodeID == node {
			keys.Keys[i].Enabled = false
		}
	}
	if action != "revoke" {
		if !Absolute(output) || output == s.NodeKeysFile || output == s.AuthFile || len(keys.Keys) >= 1024 {
			return Invalid
		}
		next, _ := Next(strconvUint(generation))
		key := ObserverKey{1, s.InstanceID, node, NewID("key"), next, NewSecret()}
		// The unpublished output cannot authenticate until the keyring replacement
		// succeeds. A failed keyring write leaves the previous complete keyring.
		if e = Save(output, key, true); e != nil {
			return Error("unsafe_file")
		}
		keys.Keys = append(keys.Keys, NodeKey{key.KeyID, node, next, Digest([]byte(key.Secret)), true, n.AssignmentRevision})
	}
	keys.Revision, e = Next(keys.Revision)
	if e != nil {
		return e
	}
	if keys.Validate(m, local) != nil {
		return Invalid
	}
	return Save(s.NodeKeysFile, keys, false)
}
func strconvUint(n uint64) string { raw, _ := json.Marshal(n); return string(raw) }
func Init(directory, instance, profile, executable string, split ...SplitOptions) (string, error) {
	if !ValidID(instance) || !OneOf(profile, "native", "docker", "mixed") {
		return "", Invalid
	}
	abs, e := filepath.Abs(directory)
	if e != nil {
		return "", Invalid
	}
	if _, e = os.Lstat(abs); e == nil {
		return "", Error("unsafe_file")
	} else if !errors.Is(e, os.ErrNotExist) {
		return "", Invalid
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(abs))
	if e != nil {
		return "", Invalid
	}
	root := filepath.Join(parent, filepath.Base(abs))
	if e = os.Mkdir(root, 0700); e != nil {
		return "", Invalid
	}
	if e = securefile.CheckDirectory(root, securefile.Private()); e != nil {
		return "", Error("unsafe_file")
	}
	for _, name := range []string{"state", "reports", "secrets", "observer-state"} {
		if e = os.Mkdir(filepath.Join(root, name), 0700); e != nil {
			return "", Invalid
		}
	}
	uid := os.Getuid()
	exe, e := filepath.Abs(executable)
	if e != nil {
		return "", Invalid
	}
	if actual, e := filepath.EvalSymlinks(exe); e == nil {
		exe = actual
	}
	m := Manifest{SchemaVersion: 1, InstanceID: instance, Revision: "1", DisplayName: "InfraPro 独立运行诊断", Nodes: []Node{{ID: "local", DisplayName: "本机", Kind: "physical", ResourceScope: "host", AssignmentRevision: "1", Enabled: true}}, Components: []Component{}, Replicas: []Replica{}, Endpoints: []Endpoint{}, Dependencies: []Dependency{}, Probes: []Probe{}}
	for _, entry := range RuntimeCatalog {
		expectation := "disabled"
		reason := "模板尚未启用；请按实际部署登记绑定"
		if entry.DefaultExpectation == "on_demand" {
			expectation = "on_demand"
			reason = entry.Reason
		}
		minReady := 0
		if entry.ID == "diagnostics-server" {
			expectation = "continuous"
			reason = entry.Reason
			minReady = 1
		}
		m.Components = append(m.Components, Component{entry.ID, entry.DisplayName, entry.Kind, expectation, minReady, reason})
		required := []string{}
		if entry.ID == "diagnostics-server" {
			required = []string{"diagnostics-process", "diagnostics-ready"}
		}
		m.Replicas = append(m.Replicas, Replica{entry.ID + "-1", entry.ID, "local", expectation, required})
	}
	m.Endpoints = append(m.Endpoints, Endpoint{"diagnostics-http", "diagnostics-server-1", "readiness", "独立诊断 HTTP"})
	m.Probes = append(m.Probes, Probe{"diagnostics-process", "local", "replica", "diagnostics-server-1", "native_process", "diagnostics-process", "presence", true, 5000, 1000, 20000}, Probe{"diagnostics-ready", "local", "endpoint", "diagnostics-http", "http_ready", "diagnostics-ready", "readiness", true, 5000, 2000, 20000}, Probe{"local-resources", "local", "node", "local", "host_resources", "local-resources", "resource", false, 10000, 1000, 30000})
	expected, _ := json.Marshal("ready")
	bindings := Bindings{SchemaVersion: 1, InstanceID: instance, NodeID: "local", AssignmentRevision: "1", Bindings: []Binding{{ID: "diagnostics-process", Adapter: "native_process", PIDFile: filepath.Join(root, "state", "process.json"), PIDFileFormat: "diagnostics", AllowedRoot: filepath.Join(root, "state"), ExpectedExecutable: exe, ExpectedUID: &uid}, {ID: "diagnostics-ready", Adapter: "http_ready", URL: "http://127.0.0.1:8096/readyz", ExpectedStatus: 200, ExpectedJSON: map[string]json.RawMessage{"status": expected}}, {ID: "local-resources", Adapter: "host_resources", Scope: "host", MountPaths: []Mount{{"diagnostics", filepath.Join(root, "state")}}, NetworkInterfaces: []NetworkInterface{}}}}
	if profile != "native" {
		m.Nodes = append(m.Nodes, Node{ID: "docker-observer", DisplayName: "独立容器观察器（待配置）", Kind: "logical_test", ResourceScope: "none", AssignmentRevision: "1", Enabled: false})
		m.Nodes = append(m.Nodes, Node{ID: "docker-vm", DisplayName: "容器宿主 VM（未接入宿主资源）", Kind: "vm", ParentNodeID: "local", ResourceScope: "vm", AssignmentRevision: "1", Enabled: false})
	}
	var remoteBindings map[string]*Bindings
	if len(split) > 0 {
		if len(split) != 1 {
			return "", Invalid
		}
		remoteBindings, e = splitTopology(&m, &bindings, profile, split[0])
		if e != nil {
			return "", e
		}
	}
	server := Server{SchemaVersion: 1, InstanceID: instance, Listen: "127.0.0.1:8096", PublicOrigin: "http://127.0.0.1:8096", AllowedHosts: []string{"127.0.0.1:8096"}, TrustedProxyCIDRs: []string{}, ManifestFile: filepath.Join(root, "manifest.json"), AuthFile: filepath.Join(root, "auth.json"), NodeKeysFile: filepath.Join(root, "node-keys.json"), StateDir: filepath.Join(root, "state"), LocalObserver: &LocalObserver{"local", "1", filepath.Join(root, "bindings.json")}, ResourceHints: ResourceHints{true, .9, 3, .1, .1, "1073741824"}}
	if m.Validate(DefaultLimits()) != nil || bindings.Validate(m) != nil || server.Validate(false) != nil {
		return "", Invalid
	}
	for node, remote := range remoteBindings {
		if e = remote.Validate(m); e != nil {
			return "", e
		}
		name := "observer-" + node + "-bindings.json"
		if node == "docker-observer" {
			name = "observer-bindings.json"
		}
		if e = Save(filepath.Join(root, name), remote, true); e != nil {
			return "", e
		}
	}
	for _, item := range []struct {
		name  string
		value any
	}{{"manifest.json", m}, {"bindings.json", bindings}, {"auth.json", Auth{1, "1", []User{}}}, {"node-keys.json", Keyring{1, instance, "1", []NodeKey{}}}, {"server.json", server}, {"runtime-catalog.json", RuntimeCatalog}} {
		if e = Save(filepath.Join(root, item.name), item.value, true); e != nil {
			return "", e
		}
	}
	return filepath.Join(root, "server.json"), nil
}
