package ingestion

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type TLSFiles struct {
	Certificate string `json:"certificate_file"`
	Key         string `json:"key_file"`
	CA          string `json:"ca_file"`
}
type RelayConfig struct {
	ID         string   `json:"id"`
	Ref        string   `json:"configuration_ref"`
	URL        string   `json:"url"`
	ConsumerID string   `json:"consumer_id"`
	Consumer   TLSFiles `json:"consumer"`
	Control    TLSFiles `json:"control"`
}
type Options struct {
	Enabled       bool          `json:"enabled"`
	StorageBudget int64         `json:"storage_budget_bytes,string"`
	QueueBudget   int           `json:"queue_budget"`
	Relays        []RelayConfig `json:"relays"`
}

func LoadOptions(path string) (Options, error) {
	var out Options
	b, e := securefile.Read(path, 1<<20, securefile.Private())
	if e != nil {
		return out, e
	}
	if e = c.StrictJSON(b, &out); e != nil {
		return out, e
	}
	if out.StorageBudget < r.MaxPullBytes || out.QueueBudget < 100 || len(out.Relays) > 20 {
		return out, c.Fail("invalid_ingestion_configuration")
	}
	ids := map[string]bool{}
	for _, v := range out.Relays {
		u, e := url.Parse(v.URL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || v.ConsumerID == "" || ids[v.ID] {
			return out, c.Fail("invalid_ingestion_configuration")
		}
		ids[v.ID] = true
	}
	return out, nil
}
func (s *Service) relay(id string) (RelayConfig, error) {
	for _, v := range s.options.Relays {
		if v.ID == id {
			return v, nil
		}
	}
	return RelayConfig{}, c.Fail("relay_configuration_unavailable")
}
func relayCall(ctx context.Context, cfg RelayConfig, control bool, path string, input, out any) error {
	f := cfg.Consumer
	if control {
		f = cfg.Control
	}
	if _, e := securefile.Read(f.Key, 64<<10, securefile.Private()); e != nil {
		return c.Fail("relay_auth_unavailable")
	}
	cert, e := tls.LoadX509KeyPair(f.Certificate, f.Key)
	if e != nil {
		return c.Fail("relay_auth_unavailable")
	}
	ca, e := os.ReadFile(f.CA)
	if e != nil {
		return c.Fail("relay_auth_unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return c.Fail("relay_auth_unavailable")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, RootCAs: roots}, ResponseHeaderTimeout: 35 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	method := "GET"
	var body io.Reader
	if input != nil {
		method = "POST"
		b, _ := json.Marshal(input)
		body = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(cfg.URL, "/")+path, body)
	if e != nil {
		return c.Fail("relay_configuration_unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := client.Do(req)
	if e != nil {
		return c.Fail("relay_unavailable")
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, r.MaxResponse+1))
	if e != nil || len(b) > r.MaxResponse {
		return c.Fail("relay_invalid_response")
	}
	if resp.StatusCode != 200 {
		var failure c.Error
		if c.StrictJSON(b, &failure) == nil {
			return &failure
		}
		return c.Fail("relay_unavailable")
	}
	if out != nil {
		return c.StrictJSON(b, out)
	}
	return nil
}
