package display

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

type ReadScopePreviewInput struct {
	Purpose    string        `json:"purpose"`
	Definition dr.Definition `json:"definition"`
}
type ReadScopeOptionsInput struct {
	StructureModel int    `json:"structure_model,omitempty"`
	Purpose        string `json:"purpose"`
	Domain         string `json:"domain"`
	ScopeID        string `json:"scope_id,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Search         string `json:"search,omitempty"`
	Cursor         string `json:"cursor,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}
type ReadScopePreview struct {
	Domain        string            `json:"domain"`
	MemberCount   string            `json:"member_count"`
	Counts        map[string]string `json:"counts"`
	Sample        []dr.ScopeOption  `json:"sample"`
	FieldGroups   []string          `json:"field_groups"`
	TimeWindow    *dr.Window        `json:"time_window,omitempty"`
	IncludeFuture bool              `json:"include_future"`
	ScopeHash     string            `json:"scope_hash"`
	Notice        string            `json:"notice"`
}

func (s *Service) WithReadOptions(domain string, reader dr.OptionsReader) *Service {
	if s.readOptions == nil {
		s.readOptions = map[string]dr.OptionsReader{}
	}
	s.readOptions[domain] = reader
	return s
}
func (s *Service) readManagementTx(ctx context.Context, a access.Actor, scope Scope, id, purpose string, fn func(pgx.Tx, Instance) error) error {
	if purpose != "config" && purpose != "grant" {
		return invalid("/purpose", "请选择 config 或 grant。")
	}
	return s.transact(ctx, a, scope, purpose, func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		v, e := instanceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		return fn(tx, v)
	})
}
func (s *Service) previewReadMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id, purpose string, d *dr.Definition) ([]dr.Member, error) {
	d.SetID, d.Digest = "", ""
	if e := dr.ValidateDefinition(d, purpose == "config", "/definition"); e != nil {
		return nil, e
	}
	members, e := s.resolveReadMembersTx(ctx, tx, a, scope, *d)
	if e != nil {
		return nil, e
	}
	if purpose == "config" {
		g, e := grantTx(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		parent, ok := findReadScope(g, d.ScopeID)
		if !ok || g.Revoked || !g.ValidUntil.After(time.Now()) || !dr.DefinitionSubset(*d, parent) {
			return nil, invalid("/definition", "配置范围或字段超出当前授权。")
		}
		allowed, e := s.effectiveReadScopeTx(ctx, tx, access.Actor{ID: g.IssuedBy}, scope, id, parent)
		if e != nil {
			return nil, e
		}
		members = dr.IntersectMembers(members, allowed)
	}
	if e = explicitReadMembers(*d, members); e != nil {
		return nil, e
	}
	return members, nil
}
func metadataGroup(domain string) string {
	return map[string]string{"resources": "resource.metadata", "channels": "resource.metadata", "devices": "device.metadata", "spatial": "spatial.metadata"}[domain]
}
func (s *Service) ReadScopePreview(ctx context.Context, a access.Actor, scope Scope, id string, in ReadScopePreviewInput) (out ReadScopePreview, err error) {
	err = s.readManagementTx(ctx, a, scope, id, in.Purpose, func(tx pgx.Tx, _ Instance) error {
		d := in.Definition
		members, e := s.previewReadMembersTx(ctx, tx, a, scope, id, in.Purpose, &d)
		if e != nil {
			return e
		}
		counts := map[string]int{}
		for _, m := range members {
			counts[m.Kind]++
		}
		out = ReadScopePreview{Domain: d.Domain, MemberCount: strconv.Itoa(len(members)), Counts: map[string]string{}, Sample: []dr.ScopeOption{}, FieldGroups: d.FieldGroups, TimeWindow: d.TimeWindow, IncludeFuture: d.IncludeFuture, ScopeHash: dr.Hash([]any{d, members}), Notice: "本项目业务资格与当前读取范围的交集；固定范围在保存时冻结。"}
		for k, v := range counts {
			out.Counts[k] = strconv.Itoa(v)
		}
		if d.IncludeFuture {
			out.Notice = "未来符合此谓词并取得项目业务资格的对象可以进入范围；配置仍须显式允许。"
		}
		if slices.Contains(d.FieldGroups, metadataGroup(d.Domain)) {
			reader := s.readOptions[d.Domain]
			if reader == nil {
				return problem("capability_unavailable", "候选目录未安装。", 409)
			}
			// Preview samples are bounded independently of the complete count. Do
			// not project every metadata row merely to display ten examples.
			sampleMembers := []dr.Member{}
			kind := map[string]string{"resources": "facility", "channels": "channel", "devices": "device", "spatial": "version"}[d.Domain]
			for _, member := range members {
				if member.Kind == kind && len(sampleMembers) < 10 {
					sampleMembers = append(sampleMembers, member)
				}
			}
			items, e := reader(ctx, tx, a, scope.Tenant, scope.Project, sampleMembers)
			if e != nil {
				return e
			}
			for _, item := range items {
				if item.Kind == map[string]string{"resources": "facility", "channels": "channel", "devices": "device", "spatial": "version"}[d.Domain] {
					out.Sample = append(out.Sample, item)
					if len(out.Sample) == 10 {
						break
					}
				}
			}
		}
		return nil
	})
	return
}
func optionKey(v dr.ScopeOption) string { return v.Kind + "\x00" + v.Code + "\x00" + v.ID }
func (s *Service) ReadScopeOptions(ctx context.Context, a access.Actor, scope Scope, id string, in ReadScopeOptionsInput) (out dr.ScopeOptionsPage, err error) {
	out.Items = []dr.ScopeOption{}
	if in.StructureModel < 0 || in.StructureModel > 2 || !dr.DomainValid(in.Domain) || utf8.RuneCountInString(in.Search) > 120 || len(in.Cursor) > 4096 || in.Limit < 0 || in.Limit > 200 {
		return out, invalid("/domain", "候选查询无效。")
	}
	if in.Limit == 0 {
		in.Limit = 50
	}
	err = s.readManagementTx(ctx, a, scope, id, in.Purpose, func(tx pgx.Tx, _ Instance) error {
		d := dr.Definition{ID: "options", StructureModel: in.StructureModel, Domain: in.Domain, Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{metadataGroup(in.Domain)}}
		if in.Purpose == "config" {
			g, e := grantTx(ctx, tx, id)
			if e != nil {
				return e
			}
			parent, ok := findReadScope(g, in.ScopeID)
			if !ok || parent.Domain != in.Domain {
				return hidden()
			}
			d = parent
			if dr.StructureVersion(in.StructureModel) > dr.StructureVersion(parent.StructureModel) {
				return invalid("/structure_model", "结构模型超出当前授权。")
			}
			d.StructureModel = in.StructureModel
			d.ID = ""
			d.ScopeID = in.ScopeID
			d.FieldGroups = []string{metadataGroup(in.Domain)}
			if in.Domain == "devices" && in.Kind == "output" {
				d.FieldGroups = append(d.FieldGroups, "device.outputs")
			}
		} else if in.Domain == "channels" {
			d.Selector.SourceKinds = []string{"direct", "derived"}
		}
		var members []dr.Member
		var e error
		if in.Purpose == "grant" {
			members, e = s.resolveReadMembersTx(ctx, tx, a, scope, d)
		} else {
			members, e = s.previewReadMembersTx(ctx, tx, a, scope, id, in.Purpose, &d)
		}
		if e != nil {
			return e
		}
		reader := s.readOptions[in.Domain]
		if reader == nil {
			return problem("capability_unavailable", "候选目录未安装。", 409)
		}
		items, e := reader(ctx, tx, a, scope.Tenant, scope.Project, members)
		if e != nil {
			return e
		}
		filterHash := dr.Hash([]any{a.ID, scope, id, in.Purpose, in.ScopeID, in.Domain, in.StructureModel, in.Kind, in.Search, members})
		var cursor struct {
			Key  string
			Hash string
		}
		if in.Cursor != "" {
			raw, e := base64.RawURLEncoding.DecodeString(in.Cursor)
			if e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Hash != filterHash {
				return problem("invalid_cursor", "候选或筛选已变化，请重新查询。", 409)
			}
		}
		slices.SortFunc(items, func(x, y dr.ScopeOption) int { return strings.Compare(optionKey(x), optionKey(y)) })
		total := 0
		search := strings.ToLower(in.Search)
		for _, item := range items {
			if in.Purpose == "config" && item.Kind == "output" && !slices.Contains(d.FieldGroups, "device.outputs") {
				continue
			}
			kindMatch := in.Kind == "" || item.Kind == in.Kind || in.Kind == "resource" && slices.Contains([]string{"facility", "component", "monitoring_section", "borehole", "point", "channel"}, item.Kind)
			if !kindMatch || search != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Code), search) {
				continue
			}
			total++
			if optionKey(item) <= cursor.Key {
				continue
			}
			if len(out.Items) < in.Limit {
				out.Items = append(out.Items, item)
			} else {
				out.HasMore = true
			}
		}
		out.Total = strconv.Itoa(total)
		if out.HasMore {
			cursor.Key = optionKey(out.Items[len(out.Items)-1])
			cursor.Hash = filterHash
			out.NextCursor = base64.RawURLEncoding.EncodeToString(dr.Bytes(cursor))
		}
		return nil
	})
	return
}

func (s *Service) WithReadDefaults(domain string, reader dr.DefaultsReader) *Service {
	if s.readDefaults == nil {
		s.readDefaults = map[string]dr.DefaultsReader{}
	}
	s.readDefaults[domain] = reader
	return s
}
