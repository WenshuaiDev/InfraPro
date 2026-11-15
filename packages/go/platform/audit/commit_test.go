package audit_test

import (
	"encoding/binary"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestAuditLostCommitAcknowledgementUsesOriginalReceipt(t *testing.T) {
	f := setup(t)
	config := f.env.Pool.Config().Copy()
	backend := net.JoinHostPort(config.ConnConfig.Host, strconv.Itoa(int(config.ConnConfig.Port)))
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	t.Cleanup(func() { _ = listener.Close() })
	var drop atomic.Bool
	go func() {
		for {
			client, e := listener.Accept()
			if e != nil {
				return
			}
			go func() {
				defer client.Close()
				server, e := net.Dial("tcp", backend)
				if e != nil {
					return
				}
				defer server.Close()
				go func() { _, _ = io.Copy(server, client) }()
				for {
					header := make([]byte, 5)
					if _, e = io.ReadFull(server, header); e != nil {
						return
					}
					n := int(binary.BigEndian.Uint32(header[1:])) - 4
					if n < 0 || n > 16*1024*1024 {
						return
					}
					body := make([]byte, n)
					if _, e = io.ReadFull(server, body); e != nil {
						return
					}
					if header[0] == 'C' && string(body) == "COMMIT\x00" && drop.CompareAndSwap(true, false) {
						return
					}
					if _, e = client.Write(append(header, body...)); e != nil {
						return
					}
				}
			}()
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	config.ConnConfig.Host = "127.0.0.1"
	config.ConnConfig.Port = uint16(address.Port)
	config.ConnConfig.TLSConfig = nil
	config.ConnConfig.Fallbacks = nil
	pool, e := pgxpool.NewWithConfig(ctx, config)
	must(t, e)
	defer pool.Close()
	must(t, pool.Ping(ctx))
	s := access.New(pool, slog.Default())
	drop.Store(true)
	m := access.Meta{CommandRequired: true, CommandID: uuid.NewString(), RequestID: uuid.NewString(), Reason: "数据库已提交但应答丢失的真实网络自验"}
	role, e := s.CreateRole(ctx, f.actor, "丢失应答只创建一次", "", m)
	must(t, e)
	if role.ID == "" || drop.Load() {
		t.Fatal("commit acknowledgement was not interrupted and resolved")
	}
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM roles WHERE name='丢失应答只创建一次'`).Scan(&count))
	if count != 1 {
		t.Fatal("ambiguous commit executed more than once")
	}
	m.RequestID = uuid.NewString()
	replayed, e := s.CreateRole(ctx, f.actor, "丢失应答只创建一次", "", m)
	must(t, e)
	if replayed.ID != role.ID {
		t.Fatal("original command receipt lost")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE object_id=$1 AND event_type='role_created'`, role.ID).Scan(&count))
	if count != 1 {
		t.Fatal("lost acknowledgement duplicated success fact")
	}
}
