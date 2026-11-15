package files

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

type fileCursor struct {
	Version                                                     int `json:"v"`
	Account, Space, Host, Filter, Authorization, Content, Epoch string
	Upper, Last                                                 time.Time
	ID                                                          string
}

func (s *Service) cursor(st txState, a access.Actor, filter any, raw string) (fileCursor, error) {
	c := fileCursor{Version: 1, Account: a.ID, Space: st.SpaceID, Host: st.HostID, Filter: taskrecord.Hash(filter), Authorization: st.AuthRevision, Content: st.ContentRevision, Epoch: st.Epoch, Upper: s.now()}
	if raw == "" {
		return c, nil
	}
	if len(raw) > 3000 {
		return c, fault("invalid_request", 400)
	}
	bytes, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil {
		return c, fault("invalid_request", 400)
	}
	var old fileCursor
	if e = json.Unmarshal(bytes, &old); e != nil || !access.ValidID(old.ID) {
		return c, fault("invalid_request", 400)
	}
	if old.Version != c.Version || old.Account != c.Account || old.Space != c.Space || old.Host != c.Host || old.Filter != c.Filter || old.Authorization != c.Authorization || old.Content != c.Content || old.Epoch != c.Epoch || old.Upper.After(s.now()) || old.Last.After(old.Upper) {
		return c, fault("cursor_stale", 409)
	}
	return old, nil
}
func nextCursor(c fileCursor, created time.Time, id string) string {
	c.Last = created
	c.ID = id
	return base64.RawURLEncoding.EncodeToString(taskrecord.JSON(c))
}
func (s *Service) limit(n int) (int, error) {
	if n == 0 {
		return s.options.ListPageSize, nil
	}
	if n < 1 || n > s.options.MaxPageSize {
		return 0, fault("invalid_request", 400)
	}
	return n, nil
}
func (s *Service) Attachments(ctx context.Context, a access.Actor, scope Scope, in AttachmentFilter) (AttachmentPage, error) {
	out := AttachmentPage{Items: []AttachmentDetail{}}
	limit, e := s.limit(in.Limit)
	if e != nil {
		return out, e
	}
	if in.Host == (HostRef{}) {
		in.Host = scope.Host()
	}
	if in.Host != scope.Host() {
		return out, fault("file_not_found", 404)
	}
	if in.State == "" {
		in.State = "active"
	}
	if in.State != "active" && in.State != "trashed" && in.State != "purged" || !validText(in.Q, 0, 200) || in.CreatedFrom != nil && in.CreatedTo != nil && in.CreatedFrom.After(*in.CreatedTo) {
		return out, fault("invalid_request", 400)
	}
	raw := in.Cursor
	in.Cursor = ""
	in.Limit = limit
	e = s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		if in.State != "active" && !st.Auth.ManagePermission {
			return access.ErrDenied
		}
		out.ContentRevision = st.ContentRevision
		out.AuthorizationRevision = st.AuthRevision
		c, e := s.cursor(st, a, in, raw)
		if e != nil {
			return e
		}
		if st.SpaceID == "" {
			return nil
		}
		q := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(in.Q)
		rows, e := tx.Query(ctx, `SELECT id::text,created_at FROM file_attachments WHERE space_id=$1 AND host_id=$2 AND status=$3 AND ($4='' OR display_name ILIKE '%'||$4||'%' ESCAPE '\') AND ($5::timestamptz IS NULL OR created_at>=$5) AND ($6::timestamptz IS NULL OR created_at<=$6) AND created_at<=$7 AND ($8::uuid IS NULL OR (created_at,id)<($9,$8)) ORDER BY created_at DESC,id DESC LIMIT $10`, st.SpaceID, st.HostID, in.State, q, in.CreatedFrom, in.CreatedTo, c.Upper, taskrecord.Null(c.ID), c.Last, limit+1)
		if e != nil {
			return e
		}
		type item struct {
			id      string
			created time.Time
		}
		ids := []item{}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.created); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for i, v := range ids {
			if i == limit {
				prev := ids[i-1]
				out.NextCursor = nextCursor(c, prev.created, prev.id)
				break
			}
			detail, e := attachmentTx(ctx, tx, st, v.id, false)
			if e != nil {
				return e
			}
			s.attachmentActions(&detail, st)
			out.Items = append(out.Items, detail)
		}
		return nil
	})
	return out, e
}
func (s *Service) Revisions(ctx context.Context, a access.Actor, scope Scope, id, cursor string, limit int) (RevisionPage, error) {
	out := RevisionPage{Items: []AttachmentRevision{}}
	limit, e := s.limit(limit)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		if _, e := attachmentTx(ctx, tx, st, id, false); e != nil {
			return e
		}
		c, e := s.cursor(st, a, map[string]any{"attachment": id, "limit": limit}, cursor)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+revisionColumns+` FROM file_attachment_revisions r JOIN file_records f ON f.id=r.file_id WHERE r.space_id=$1 AND r.attachment_id=$2 AND r.created_at<=$3 AND ($4::uuid IS NULL OR (r.created_at,r.id)<($5,$4)) ORDER BY r.created_at DESC,r.id DESC LIMIT $6`, st.SpaceID, id, c.Upper, taskrecord.Null(c.ID), c.Last, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v AttachmentRevision
			if e = rows.Scan(revisionScan(&v)...); e != nil {
				return e
			}
			if len(out.Items) == limit {
				last := out.Items[len(out.Items)-1]
				out.NextCursor = nextCursor(c, last.CreatedAt, last.ID)
				break
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Uploads(ctx context.Context, a access.Actor, scope Scope, in UploadFilter) (UploadPage, error) {
	out := UploadPage{Items: []UploadDetail{}}
	limit, e := s.limit(in.Limit)
	if e != nil {
		return out, e
	}
	if in.View == "" {
		in.View = "mine"
	}
	if in.View != "mine" && in.View != "managed" {
		return out, fault("invalid_request", 400)
	}
	if in.State != "" && in.State != "receiving" && in.State != "submitted" && in.State != "closed" && in.State != "cancelled" && in.State != "expired" {
		return out, fault("invalid_request", 400)
	}
	raw := in.Cursor
	in.Cursor = ""
	in.Limit = limit
	e = s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		if in.View == "managed" && !st.Auth.ManagePermission {
			return access.ErrDenied
		}
		c, e := s.cursor(st, a, in, raw)
		if e != nil {
			return e
		}
		if st.SpaceID == "" {
			return nil
		}
		rows, e := tx.Query(ctx, `SELECT id::text,created_at FROM file_uploads WHERE space_id=$1 AND host_id=$2 AND ($3='managed' OR actor_id=$4) AND ($5='' OR state=$5) AND created_at<=$6 AND ($7::uuid IS NULL OR (created_at,id)<($8,$7)) ORDER BY created_at DESC,id DESC LIMIT $9`, st.SpaceID, st.HostID, in.View, a.ID, in.State, c.Upper, taskrecord.Null(c.ID), c.Last, limit+1)
		if e != nil {
			return e
		}
		type item struct {
			id      string
			created time.Time
		}
		ids := []item{}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.created); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for i, v := range ids {
			if i == limit {
				prev := ids[i-1]
				out.NextCursor = nextCursor(c, prev.created, prev.id)
				break
			}
			u, e := uploadTx(ctx, tx, st.SpaceID, v.id, false)
			if e != nil {
				return e
			}
			var j JobSummary
			e = tx.QueryRow(ctx, `SELECT id::text,generation::text,control_revision::text,state,phase FROM file_jobs WHERE upload_id=$1`, v.id).Scan(&j.ID, &j.Generation, &j.ControlRevision, &j.State, &j.Phase)
			if e != nil && e != pgx.ErrNoRows {
				return e
			}
			if e == nil {
				j.TaskRef = jobRef(j.ID, j.Generation)
				u.Job = &j
			}
			e = tx.QueryRow(ctx, `SELECT unbound_until FROM file_records WHERE upload_id=$1`, v.id).Scan(&u.PreparedUntil)
			if e != nil && e != pgx.ErrNoRows {
				return e
			}
			out.Items = append(out.Items, u.UploadDetail)
		}
		return nil
	})
	return out, e
}
