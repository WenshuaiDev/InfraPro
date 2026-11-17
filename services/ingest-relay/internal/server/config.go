package server

import (
	"crypto/tls"
	"crypto/x509"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"os"
	"slices"
	"time"
)

type Principal struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	ConsumerID string `json:"consumer_id,omitempty"`
}
type Config struct {
	RelayID         string   `json:"relay_id"`
	DatabaseURL     string   `json:"database_url"`
	Address         string   `json:"control_address"`
	Certificate     string   `json:"certificate_file"`
	Key             string   `json:"key_file"`
	CA              string   `json:"client_ca_file"`
	PrincipalsFile  string   `json:"principals_file"`
	Budget          int64    `json:"budget_bytes,string"`
	PortMin         int      `json:"listener_port_min"`
	PortMax         int      `json:"listener_port_max"`
	ListenAddresses []string `json:"listen_addresses"`
	MaxConnections  int      `json:"max_connections"`
}

func privateFile(path string) ([]byte, error) {
	f, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !f.Mode().IsRegular() || f.Mode().Perm()&0077 != 0 || f.Size() > 1<<20 {
		return nil, c.Fail("private_configuration_required")
	}
	return os.ReadFile(path)
}
func ReadConfig(path string) (Config, error) {
	var v Config
	b, e := privateFile(path)
	if e != nil {
		return v, e
	}
	if e = c.StrictJSON(b, &v); e != nil {
		return v, e
	}
	if v.Address == "" || v.DatabaseURL == "" || v.Budget < 4<<20 || v.PortMin < 1024 || v.PortMax < v.PortMin || v.PortMax > 65535 || len(v.ListenAddresses) == 0 || v.MaxConnections < 1 || v.MaxConnections > 10000 {
		return v, c.Fail("invalid_configuration")
	}
	return v, nil
}
func (cfg Config) tlsConfig() (*tls.Config, error) {
	if _, e := privateFile(cfg.Key); e != nil {
		return nil, e
	}
	cert, e := tls.LoadX509KeyPair(cfg.Certificate, cfg.Key)
	if e != nil {
		return nil, e
	}
	ca, e := os.ReadFile(cfg.CA)
	if e != nil {
		return nil, e
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, c.Fail("invalid_ca")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}, nil
}
func (cfg Config) TLS() (*tls.Config, error) {
	t, e := cfg.tlsConfig()
	if e != nil {
		return nil, e
	}
	t.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) { return cfg.tlsConfig() }
	return t, nil
}
func (cfg Config) Principal(state *tls.ConnectionState, roles ...string) (Principal, error) {
	var p Principal
	if state == nil || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
		return p, c.Fail("machine_auth_failed")
	}
	for _, chain := range state.VerifiedChains {
		for _, cert := range chain {
			if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
				return p, c.Fail("machine_auth_failed")
			}
		}
	}
	b, e := privateFile(cfg.PrincipalsFile)
	if e != nil {
		return p, e
	}
	var list map[string]Principal
	if c.StrictJSON(b, &list) != nil {
		return p, c.Fail("machine_auth_failed")
	}
	p, ok := list[c.Digest(state.PeerCertificates[0].Raw)]
	if !ok || p.ID == "" || !slices.Contains(roles, p.Role) {
		return Principal{}, c.Fail("machine_auth_failed")
	}
	return p, nil
}
