package displayread

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type SetOwner struct {
	Tenant, Project, Instance, Kind, Config, Name string
	GrantRevision                                 int64
}

func FreezeTx(ctx context.Context, tx pgx.Tx, owner SetOwner, definition *Definition, members []Member) error {
	var e error
	members, e = NormalizeMembers(members)
	if e != nil {
		return e
	}
	definition.SetID = ""
	definition.Digest = Hash(members)
	var config, revision any
	if owner.Kind == "config" {
		config = owner.Config
	} else {
		revision = owner.GrantRevision
	}
	e = tx.QueryRow(ctx, `INSERT INTO display_read_scope_sets(tenant_id,project_id,instance_id,domain,owner_kind,config_id,grant_revision,name,definition,digest,member_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, owner.Tenant, owner.Project, owner.Instance, definition.Domain, owner.Kind, config, revision, owner.Name, Bytes(definition), definition.Digest, len(members)).Scan(&definition.SetID)
	if e != nil {
		return e
	}
	_, e = tx.CopyFrom(ctx, pgx.Identifier{"display_read_scope_members"}, []string{"set_id", "domain", "kind", "object_id"}, pgx.CopyFromSlice(len(members), func(i int) ([]any, error) {
		return []any{definition.SetID, definition.Domain, members[i].Kind, members[i].ID}, nil
	}))
	if e != nil {
		return e
	}
	if owner.Kind == "config" {
		_, e = tx.Exec(ctx, `INSERT INTO display_read_config_scopes(tenant_id,project_id,instance_id,config_id,name,set_id) VALUES($1,$2,$3,$4,$5,$6)`, owner.Tenant, owner.Project, owner.Instance, owner.Config, owner.Name, definition.SetID)
	}
	return e
}
func MembersTx(ctx context.Context, tx pgx.Tx, tenant, project, instance string, d Definition) ([]Member, error) {
	var digest string
	var count int
	if e := tx.QueryRow(ctx, `SELECT digest,member_count FROM display_read_scope_sets WHERE tenant_id=$1 AND project_id=$2 AND instance_id=$3 AND id=$4 AND domain=$5`, tenant, project, instance, d.SetID, d.Domain).Scan(&digest, &count); e != nil {
		if e == pgx.ErrNoRows {
			return nil, Hidden()
		}
		return nil, e
	}
	if digest != d.Digest {
		return nil, Problem("scope_unavailable", "固定成员依据不匹配。", 409)
	}
	rows, e := tx.Query(ctx, `SELECT object_id::text,kind FROM display_read_scope_members WHERE set_id=$1 ORDER BY kind,object_id`, d.SetID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Member, 0, count)
	for rows.Next() {
		var m Member
		if e = rows.Scan(&m.ID, &m.Kind); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) != count || Hash(out) != digest {
		return nil, Problem("scope_unavailable", "固定成员集合不完整。", 409)
	}
	return out, nil
}
func Count(members []Member) string { return strconv.Itoa(len(members)) }
