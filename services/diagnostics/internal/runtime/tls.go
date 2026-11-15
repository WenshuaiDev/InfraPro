package runtime

import (
	"crypto/tls"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"net/http"
)

func installTLS(server *http.Server, cert, key []byte) error {
	pair, e := tls.X509KeyPair(cert, key)
	if e != nil {
		return settings.Invalid
	}
	server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	return nil
}
