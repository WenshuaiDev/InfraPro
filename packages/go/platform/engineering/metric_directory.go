package engineering

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

func metricDirectoryTx(ctx context.Context, tx pgx.Tx, scope Scope, filter Filter, page *Page) error {
	const directory = `, entries AS (
		SELECT d.id, d.code, d.created_at, COALESCE(v.number,0) AS latest, v.id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM blocked WHERE id=v.id) AS latest_usable, COALESCE(v.content,dr.content) AS content,
			dr.definition_id IS NOT NULL AS has_draft, dr.content->>'name' AS draft_name, COALESCE(d.source_version::text,'') AS source_version,
			EXISTS(SELECT 1 FROM engineering_versions ev WHERE ev.definition_id=d.id AND NOT EXISTS(SELECT 1 FROM blocked b WHERE b.id=ev.id)) AS usable
		FROM engineering_definitions d LEFT JOIN engineering_drafts dr ON dr.definition_id=d.id
		LEFT JOIN LATERAL (SELECT id,number,content FROM engineering_versions WHERE definition_id=d.id ORDER BY number DESC LIMIT 1) v ON true
		WHERE d.scope_key=$1 AND d.kind='metrics' AND d.deleted_at IS NULL
	), matched AS (
		SELECT entries.*, CASE WHEN jsonb_typeof(content#>'{metric,facility_topics}')='array'
			THEN ARRAY(SELECT DISTINCT tag FROM jsonb_array_elements_text(content#>'{metric,facility_topics}') tag WHERE tag <> '')
			ELSE ARRAY(SELECT DISTINCT btrim(tag) FROM regexp_split_to_table(substring(content->>'description' FROM '(?n)^[[:blank:]]*适用设施[：:][[:blank:]]*([^。\r\n]*)'),'[、，,；;]+') tag WHERE btrim(tag)<>'') END AS topics
		FROM entries WHERE ($2='' OR position(lower($2) in lower(code))>0 OR position(lower($2) in lower(content->>'name'))>0 OR position(lower($2) in lower(draft_name))>0)
		AND ($3='' OR ($3='draft' AND has_draft) OR ($3='usable' AND usable) OR ($3='unavailable' AND NOT usable))
	), selected AS (
		SELECT * FROM matched WHERE $4='' OR $4=ANY(topics) OR ($4='__unassigned' AND cardinality(topics)=0)
		ORDER BY created_at DESC,id DESC LIMIT $5 OFFSET $6
	)
	SELECT (SELECT jsonb_object_agg(tag,total) FROM (
		SELECT '__all' AS tag,count(*) AS total FROM matched UNION ALL
		SELECT tag,count(*) FROM matched CROSS JOIN LATERAL unnest(CASE WHEN cardinality(topics)=0 THEN ARRAY['__unassigned'] ELSE topics END) tag GROUP BY tag
	) counts), COALESCE((SELECT jsonb_agg(jsonb_build_object(
		'id',s.id,'kind','metrics','code',s.code,'name',COALESCE(s.content->>'name',''),'latest',s.latest,'usable',s.usable,'has_draft',s.has_draft,'source_version',s.source_version,
		'metric_index',jsonb_build_object('latest_usable',s.latest_usable,'shape',COALESCE(s.content#>>'{metric,shape}',''),'standard_symbol',COALESCE(u.content#>>'{unit,symbol}',''),'facility_topics',s.topics)
	) ORDER BY s.created_at DESC,s.id DESC) FROM selected s LEFT JOIN engineering_versions u ON u.id=NULLIF(s.content#>>'{metric,standard_unit}','')::uuid),'[]')`
	err := tx.QueryRow(ctx, blockedCTE+directory, scope.key(), strings.TrimSpace(filter.Query), filter.Status, filter.FacilityTopic, page.Limit, page.Offset).Scan(&page.FacilityCounts, &page.Items)
	topic := filter.FacilityTopic
	if topic == "" {
		topic = "__all"
	}
	page.Total = page.FacilityCounts[topic]
	return err
}

func unitChoicesTx(ctx context.Context, tx pgx.Tx, scope Scope, filter Filter, page *Page) error {
	const choices = `, matching AS (
		SELECT d.id,d.code,max(v.number) AS latest,jsonb_agg(jsonb_build_object('id',v.id,'number',v.number,'name',v.content->>'name',
			'symbol',v.content#>>'{unit,symbol}','category',v.content#>>'{unit,category}') ORDER BY v.number DESC) AS versions
		FROM engineering_definitions d JOIN engineering_versions v ON v.definition_id=d.id
		WHERE d.scope_key=$1 AND d.kind='units' AND d.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM blocked WHERE blocked.id=v.id)
			AND ($2='' OR v.content#>>'{unit,category}'=$2) AND ($3='' OR position(lower($3) in lower(d.code))>0
			OR position(lower($3) in lower(v.content->>'name'))>0 OR position(lower($3) in lower(v.content#>>'{unit,symbol}'))>0)
		GROUP BY d.id,d.code
	), selected AS (SELECT * FROM matching ORDER BY code,id LIMIT $4 OFFSET $5)
	SELECT (SELECT count(*) FROM matching),COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'kind','units','code',code,'name',versions#>>'{0,name}',
		'latest',latest,'has_draft',false,'usable',true,'source_version','','unit_versions',versions) ORDER BY code,id) FROM selected),'[]')`
	return tx.QueryRow(ctx, blockedCTE+choices, scope.key(), filter.UnitCategory, strings.TrimSpace(filter.Query), page.Limit, page.Offset).Scan(&page.Total, &page.Items)
}
