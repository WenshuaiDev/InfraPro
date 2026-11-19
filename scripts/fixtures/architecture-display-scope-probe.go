// This fixture calls the same domain methods used by trusted display pairing.
// The caller supplies database connections over stdin, never in process arguments.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type input struct {
	URL, Role string
	Projects  []project
}
type project struct{ Tenant, Project, Account string }
type resolve func(context.Context, pgx.Tx, access.Actor, string, string, dr.Selector) ([]dr.Member, error)
type result struct {
	Count  int            `json:"count"`
	SHA    string         `json:"sha256"`
	Kinds  map[string]int `json:"kinds,omitempty"`
	Code   string         `json:"code,omitempty"`
	Reason string         `json:"reason,omitempty"`
}
type scene struct {
	Project  string `json:"project"`
	Domain   string `json:"domain"`
	Selector string `json:"selector"`
	Result   result `json:"result"`
}

func errResult(err error) result {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		return result{Code: p.Code, Reason: p.Message}
	}
	var domain *access.Error
	if errors.As(err, &domain) {
		return result{Code: domain.Code}
	}
	var business interface{ StatusCode() int }
	if errors.As(err, &business) {
		return result{Code: fmt.Sprintf("HTTP_%d", business.StatusCode()), Reason: err.Error()}
	}
	return result{Code: "unexpected", Reason: err.Error()}
}

func summary(v any, n int) result {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(b)
	return result{Count: n, SHA: hex.EncodeToString(digest[:])}
}

func probe(ctx context.Context, args input) ([]scene, error) {
	db, err := pgx.Connect(ctx, args.URL)
	if err != nil {
		return nil, fmt.Errorf("probe database connection failed")
	}
	defer db.Close(ctx)
	var login string
	var elevated bool
	err = db.QueryRow(ctx, `SELECT current_user,rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=session_user AND current_user=session_user`).Scan(&login, &elevated)
	if err != nil || (args.Role != "owner" && elevated) {
		return nil, fmt.Errorf("unexpected role authentication or bypass")
	}
	monitor := monitoring.New(nil)
	derived := derivedmonitoring.New(nil).WithMonitoring(monitor)
	monitor.WithDisplayQualification(derived.QualifyDisplayInputsTx)
	space := new(spatial.Service)
	resolvers := map[string]resolve{"resources": new(resources.Service).DisplayMembersTx, "devices": new(devices.Service).DisplayMembersTx, "monitoring": monitor.DisplayMembersTx, "spatial": space.DisplayMembersTx}
	out := []scene{}
	for _, p := range args.Projects {
		// This is the stored grantor's guard from display.resolveReadMembersTx.
		actor := access.Actor{ID: p.Account, Check: func(c context.Context, t pgx.Tx, recent bool) error {
			if recent {
				return access.ErrDenied
			}
			state, e := access.ProjectSnapshotTx(c, t, p.Account, p.Tenant, p.Project, "")
			if e != nil {
				return e
			}
			if len(state.InactiveReasons) != 0 {
				return access.ErrDenied
			}
			return nil
		}}
		for _, name := range []string{"resources", "devices", "monitoring", "spatial"} {
			res := resolvers[name]
			var first []dr.Member
			for _, step := range []string{"all", "empty", "unauthorized", "explicit", "defaults", "derived-qualification"} {
				if step == "defaults" && name != "spatial" || step == "derived-qualification" && name != "monitoring" {
					continue
				}
				if step == "explicit" && len(first) == 0 {
					continue
				}
				value := func() result {
					tx, e := db.Begin(ctx)
					if e != nil {
						return errResult(e)
					}
					defer tx.Rollback(ctx)
					a := actor
					sel := dr.Selector{Kind: "filter", StructureModel: 2}
					switch step {
					case "empty":
						sel.Kind = "ids"
						sel.IDs = []string{"00000000-0000-4000-8000-000000000001"}
					case "unauthorized":
						a = access.Actor{ID: "00000000-0000-4000-8000-000000000002"}
					case "explicit":
						sel.Kind = "ids"
						sel.IDs = []string{first[0].ID}
						if name == "devices" && first[0].Kind == "output" {
							sel.OutputIDs = sel.IDs
							sel.IDs = nil
						}
					case "defaults":
						v, e := space.DisplayDefaultsTx(ctx, tx, a, p.Tenant, p.Project, first)
						if e != nil {
							return errResult(e)
						}
						return summary(v, len(v.Items))
					case "derived-qualification":
						sel.SourceKinds = []string{"derived"}
					}
					members, e := res(ctx, tx, a, p.Tenant, p.Project, sel)
					if e != nil {
						return errResult(e)
					}
					if step == "all" {
						first = members
					}
					if step == "derived-qualification" {
						// Qualify each visible derived channel separately: a withdrawn input is
						// an authorization outcome, not a database permission exception.
						permitted := []dr.Member{}
						denials := map[string]int{}
						for _, m := range members {
							e = monitor.QualifyDisplayMembersTx(ctx, tx, a, p.Tenant, p.Project, dr.Definition{Domain: "monitoring"}, []dr.Member{m})
							if e != nil {
								r := errResult(e)
								var sqlFailure *pgconn.PgError
								if r.Code == "unexpected" || errors.As(e, &sqlFailure) {
									return r
								}
								denials[r.Code]++
								continue
							}
							permitted = append(permitted, m)
						}
						v := summary(permitted, len(permitted))
						v.Kinds = denials
						v.Kinds["visibleDerived"] = len(members)
						return v
					}
					v := summary(members, len(members))
					v.Kinds = map[string]int{}
					for _, m := range members {
						v.Kinds[m.Kind]++
					}
					return v
				}()
				out = append(out, scene{p.Project, name, step, value})
			}
		}
	}
	return out, nil
}
func main() {
	var args input
	if err := json.NewDecoder(os.Stdin).Decode(&args); err != nil {
		fmt.Fprintln(os.Stderr, "Invalid probe input")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	scenes, err := probe(ctx, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(scenes); err != nil {
		panic(err)
	}
}
