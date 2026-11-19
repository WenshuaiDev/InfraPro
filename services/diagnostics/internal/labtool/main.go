// labtool is an isolated acceptance helper, never embedded in the diagnostics
// server. It emits no secrets and does not change the system certificate store.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/adapters"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/shirou/gopsutil/v4/process"
)

type processIdentity struct {
	PID        int32  `json:"pid"`
	Started    int64  `json:"os_start_time_ms"`
	Executable string `json:"executable"`
	UID        int32  `json:"uid"`
}

func identity(pid string) (processIdentity, error) {
	v, e := strconv.ParseInt(pid, 10, 32)
	if e != nil || v <= 1 {
		return processIdentity{}, settings.Invalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, e := process.NewProcessWithContext(ctx, int32(v))
	if e != nil {
		return processIdentity{}, e
	}
	started, e := p.CreateTimeWithContext(ctx)
	if e != nil {
		return processIdentity{}, e
	}
	exe, e := p.ExeWithContext(ctx)
	if e != nil {
		return processIdentity{}, e
	}
	uids, e := p.UidsWithContext(ctx)
	if e != nil || len(uids) == 0 {
		return processIdentity{}, settings.Invalid
	}
	return processIdentity{int32(v), started, exe, int32(uids[0])}, nil
}
func run() error {
	args := os.Args[1:]
	if len(args) == 0 {
		return settings.Invalid
	}
	switch args[0] {
	case "probe":
		if len(args) != 4 {
			return settings.Invalid
		}
		var m settings.Manifest
		var b settings.Bindings
		if _, err := settings.Load(args[1], &m); err != nil || m.Validate(settings.DefaultLimits()) != nil {
			return settings.Invalid
		}
		if _, err := settings.Load(args[2], &b); err != nil || b.Validate(m) != nil {
			return settings.Invalid
		}
		for _, p := range m.Assigned(b.NodeID) {
			if p.ID != args[3] {
				continue
			}
			binding, ok := b.Find(p.BindingID)
			if !ok {
				return settings.Invalid
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.TimeoutMS)*time.Millisecond)
			defer cancel()
			return json.NewEncoder(os.Stdout).Encode(adapters.New(m, b, uuid.NewString()).Collect(ctx, p, binding))
		}
		return settings.Invalid
	case "identity":
		if len(args) != 2 {
			return settings.Invalid
		}
		v, e := identity(args[1])
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "signal":
		if len(args) != 3 {
			return settings.Invalid
		}
		var prior processIdentity
		if _, e := settings.Load(args[1], &prior); e != nil {
			return e
		}
		current, e := identity(strconv.Itoa(int(prior.PID)))
		if e != nil {
			return e
		}
		if current != prior || int(current.UID) != os.Getuid() {
			return settings.Error("identity_mismatch")
		}
		signals := map[string]syscall.Signal{"TERM": syscall.SIGTERM, "KILL": syscall.SIGKILL, "STOP": syscall.SIGSTOP, "CONT": syscall.SIGCONT}
		sig, ok := signals[args[2]]
		if !ok {
			return settings.Invalid
		}
		return syscall.Kill(int(current.PID), sig)
	case "auth":
		if len(args) != 3 {
			return settings.Invalid
		}
		cfg, e := settings.LoadServer(args[1])
		if e != nil {
			return e
		}
		password := settings.NewSecret()
		if e = settings.EditAuth(args[1], "init", "operator", password, nil); e != nil {
			return e
		}
		return settings.Save(args[2], map[string]any{"synthetic": true, "origin": cfg.PublicOrigin, "username": "operator", "password": password}, true)
	case "cert":
		if len(args) != 2 && len(args) != 3 {
			return settings.Invalid
		}
		variant := "valid"
		if len(args) == 3 {
			variant = args[2]
		}
		return cert(args[1], variant)
	case "buckets":
		if len(args) != 3 {
			return settings.Invalid
		}
		u, e := url.Parse(args[1])
		if e != nil || u.Hostname() != "127.0.0.1" || u.Scheme != "http" {
			return settings.Invalid
		}
		var c settings.Credential
		if _, e = settings.Load(args[2], &c); e != nil || c.Validate() != nil || c.Kind != "s3" {
			return settings.Invalid
		}
		client, e := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath})
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, bucket := range []string{"diagnostics-source", "diagnostics-ch"} {
			exists, e := client.BucketExists(ctx, bucket)
			if e != nil {
				return e
			}
			if !exists {
				if e = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); e != nil {
					return e
				}
			}
		}
		return nil
	default:
		return settings.Invalid
	}
}
func cert(dir, variant string) error {
	if !settings.OneOf(variant, "valid", "expired", "hostname") {
		return settings.Invalid
	}
	if securefile.CheckDirectory(dir, securefile.Private()) != nil {
		return settings.Invalid
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	ca := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "InfraPro isolated diagnostics lab CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		return e
	}
	if e = securefile.Atomic(filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), securefile.Private(), true); e != nil {
		return e
	}
	leafKey, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	serial, _ = rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	leaf := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "diagnostics-lab"}, DNSNames: []string{"localhost", "host.docker.internal", "diagnostics"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if variant == "expired" {
		leaf.NotBefore = time.Now().Add(-2 * time.Hour)
		leaf.NotAfter = time.Now().Add(-time.Hour)
	}
	if variant == "hostname" {
		leaf.DNSNames = []string{"wrong.invalid"}
		leaf.IPAddresses = nil
	}
	der, e = x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if e != nil {
		return e
	}
	private, e := x509.MarshalPKCS8PrivateKey(leafKey)
	if e != nil {
		return e
	}
	if e = securefile.Atomic(filepath.Join(dir, "server-key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), securefile.Private(), true); e != nil {
		return e
	}
	return securefile.Atomic(filepath.Join(dir, "server.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), securefile.Private(), true)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "diagnostics lab helper:", settings.Code(e))
		os.Exit(1)
	}
}
