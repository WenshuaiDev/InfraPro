import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { Permissions as P, type DisplayBinding, type DisplayConfig, type DisplayDetail, type DisplayGrant, type DisplayList, type DisplayPackage, type DisplayReadDefinition } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { useProject } from '../projects/context';
import { BindingEditor, SchemaField, fieldClass, ObjectPicker } from './forms';
import { upgradeConfig } from './config-upgrade';
import { ReadScopeEditor, ReadViewEditors } from './read-scope-editor';
import {newDefinition,domainLabels} from './read-scope-model';

export function Configuration({
  data,
  busy,
  save,
  runtime,
  packageID,
}: {
  data: DisplayDetail;
  busy: boolean;
  save: (body: object) => Promise<unknown>;
  runtime: string;
  packageID?: string;
}) {
  const project = useProject();
  const prior = data.configs?.find(c => c.id === data.instance.draft_config_id);
  const initialPackage = data.packages?.find(p => p.id === (packageID ?? prior?.package_id) && p.state === 'ready');
  const [initial] = useState(() => initialPackage ? upgradeConfig(initialPackage, prior) : undefined);
  const [selected, setSelected] = useState(initialPackage?.id ?? ""),
    [parameters, setParameters] = useState<Record<string, unknown>>(initial?.parameters ?? {}),
    [bindings, setBindings] = useState<Record<string, DisplayBinding>>(initial?.bindings ?? {}),
    [readViews,setReadViews] = useState<Record<string,DisplayReadDefinition>>(initial?.read_views??{}),
    [mode, setMode] = useState<DisplayConfig["mode"]>(initial?.mode ?? "fixed"),
    [upgrade, setUpgrade] = useState<ReturnType<typeof upgradeConfig> | undefined>(packageID && prior ? initial : undefined);
  const p = data.packages?.find((p) => p.id === selected);
  function choose(p: DisplayPackage, c?: DisplayConfig) {
    const prior = c ?? (selected ? {parameters, bindings, mode,read_views:readViews} : data.configs?.find(c => c.id === data.instance.draft_config_id));
    const result = upgradeConfig(p, prior);
    setSelected(p.id);
    setParameters(result.parameters);
    setBindings(result.bindings);
    setReadViews(result.read_views);
    setMode(result.mode);
    setUpgrade(prior ? result : undefined);
  }
  return (
    <div className="space-y-5">
      <fieldset disabled={busy} className="space-y-5">
      <label className="block text-sm font-medium">
        程序版本
        <select
          aria-label="选择程序版本"
          className={fieldClass}
          value={selected}
          onChange={(e) => {
            const p = data.packages?.find((p) => p.id === e.target.value);
            if (p) choose(p);
          }}
        >
          <option value="">请选择已就绪包</option>
          {data.packages
            ?.filter((p) => p.state === "ready")
            .map((p) => (
              <option key={p.id} value={p.id}>
                {p.manifest.name ?? p.manifest.app_key} · {p.manifest.version} ·{" "}
                {p.id.slice(0, 8)}
              </option>
            ))}
        </select>
      </label>
      {p && (
        <>
          {upgrade && <div className="space-y-1 rounded-lg border bg-muted p-3 text-sm" role="status">
            <p>版本切换时：继承 {upgrade.inherited.length} 项，重置 {upgrade.replaced.length} 项，移除 {upgrade.removed.length} 项。</p>
            {upgrade.missing.length > 0 && <p>待补充：{upgrade.missing.join("、")}</p>}
            {upgrade.replaced.length > 0 && <p>类型或约束不兼容：{upgrade.replaced.join("、")}</p>}
            {upgrade.removed.length > 0 && <p>新版本不再使用：{upgrade.removed.join("、")}</p>}
            <p className="text-muted-foreground">请检查参数与绑定后保存为新修订。绑定的当前资格和业务语义将在预览、发布准备时再次核验。</p>
          </div>}
          <SchemaField
            schema={p.manifest.config_schema}
            value={parameters}
            onChange={(v) => setParameters(v as Record<string, unknown>)}
            label="普通参数"
          />
          {!!Object.keys(p.manifest.read_view_schema??{}).length&&<ReadViewEditors data={data} schema={p.manifest.read_view_schema??{}} values={readViews} onChange={setReadViews}/>}
          {!!Object.keys(p.manifest.binding_schema).length&&<div className="space-y-1"><h3 className="text-sm font-semibold">旧绑定访问契约</h3><p className="text-xs text-muted-foreground">这些绑定继续使用原有字段授权。命名视图收窄字段不会改变旧绑定的可读数据。</p></div>}
          {Object.entries(p.manifest.binding_schema).map(([key, s]) => (
            <div key={key} className="space-y-2">
              <p className="text-sm">
                {s.required ? "必需" : "可选"} · {s.kind} · 数量{" "}
                {s.min_items ?? 0}–{s.max_items ?? 500}
                {s.value_kind ? ` · ${s.value_kind}` : ""}
                {s.source_kind ? ` · ${s.source_kind}` : ""}
              </p>
              <BindingEditor
                label={key}
                value={bindings[key] ?? { kind: s.kind, ids: [] }}
                onChange={(v) => setBindings({ ...bindings, [key]: v })}
              />
            </div>
          ))}
          <label className="block text-sm">
            时间模式
            <select
              aria-label="时间模式"
              className={fieldClass}
              value={mode}
              onChange={(e) => setMode(e.target.value === "follow" ? "follow" : "fixed")}
            >
              <option value="fixed">固定历史范围</option>
              <option value="follow">跟随时间窗口</option>
            </select>
          </label>
          <Button
            disabled={busy}
            onClick={() =>
              void save({ package_id: p.id, parameters, bindings, mode, config_format:p.manifest.manifest_version===2?2:1, ...(p.manifest.manifest_version===2?{read_views:readViews}:{}) }).catch(
                () => {},
              )
            }
          >
            保存草稿
          </Button>
        </>
      )}
      </fieldset>
      <h3 className="text-sm font-semibold">保存的配置</h3>
      {data.configs?.map((c) => (
        <div
          key={c.id}
          className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3 text-sm"
        >
          <span>
            {c.id === data.instance.draft_config_id ? "当前草稿" : "历史配置"} ·{" "}
            {new Date(c.created_at).toLocaleString()}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => {
                const p = data.packages?.find((p) => p.id === c.package_id);
                if (p) choose(p, c);
              }}
            >
              作为编辑起点
            </Button>
            {project.can(P.ProjectDisplayPreview) && <a
              className="self-center text-primary underline"
              href={`${runtime}/display/${data.instance.id}?config=${c.id}`}
              target="_blank"
              rel="noreferrer"
            >
              真实预览
            </a>}
          </div>
        </div>
      ))}
    </div>
  );
}
export function GrantForm({
  data,
  capabilities,
  disabled,
  archived,
  save,
}: {
  data: DisplayDetail;
  capabilities: DisplayList["capabilities"];
  disabled: boolean;
  archived: boolean;
  save: (grant: DisplayGrant) => Promise<unknown>;
}) {
  const [grant, setGrant] = useState<DisplayGrant>(
    () =>
      data.grant ?? {
        capabilities: [],
        scope: { channels: [], resources: [], spatial: [], artifacts: [] },
        fields: ["metadata", "values", "quality"],
        valid_until: new Date(Date.now() + 90 * 86400000).toISOString(),
        revoked: false,
      },
  );
  const update = (v: Partial<DisplayGrant>) => setGrant({ ...grant, ...v });
  return (
    <fieldset disabled={disabled} className="space-y-5">
      <p className="text-sm text-muted-foreground">
        读取授权限定此实例可使用的能力、对象、字段与期限。包内绑定还需落在此范围内。
      </p>
      <fieldset className="grid gap-2 rounded-lg border p-4">
        <legend className="px-2">业务能力</legend>
        {capabilities.map((c) => (
          <label key={c.name} className="flex items-start gap-2 text-sm">
            <input
              type="checkbox"
              checked={grant.capabilities.includes(c.name)}
              disabled={c.availability === "unavailable"}
              onChange={(e) =>
                update({
                  capabilities: e.target.checked
                    ? [...grant.capabilities, c.name]
                    : grant.capabilities.filter((v) => v !== c.name),
                })
              }
            />
            <span>
              {c.name}
              {c.availability === "unavailable" && (
                <span className="ml-2 text-muted-foreground">
                  未接入：{c.reason}
                </span>
              )}
            </span>
          </label>
        ))}
      </fieldset>
      <section className="space-y-4"><h3 className="text-sm font-semibold">命名读取范围</h3><p className="text-xs text-muted-foreground">授权收窄后，现有视图和终端会在下一次读取前重新校验；被移除的范围不能继续供数。</p>{grant.read_scopes?.map((scope,index)=><fieldset className="space-y-3 rounded-lg border p-4" key={index}><ReadScopeEditor data={data} purpose="grant" value={scope} onChange={value=>update({scope_format:2,read_scopes:grant.read_scopes?.map((s,i)=>i===index?value:s)})}/><Button type="button" variant="ghost" onClick={()=>update({read_scopes:grant.read_scopes?.filter((_,i)=>i!==index)})}>移除此授权范围</Button></fieldset>)}<div className="flex flex-wrap gap-2">{Object.entries(domainLabels).map(([domain,label])=><Button key={domain} type="button" variant="outline" disabled={archived||(grant.read_scopes?.length??0)>=64} onClick={()=>{const id=`${domain}_${(grant.read_scopes?.length??0)+1}`;update({scope_format:2,read_scopes:[...grant.read_scopes??[],newDefinition(domain as DisplayReadDefinition['domain'],id)]});}}>添加{label}范围</Button>)}</div></section>
      <details open={grant.scope_format!==2} className="space-y-4 rounded-lg border p-4"><summary className="cursor-pointer text-sm font-semibold">旧绑定访问契约</summary><p className="text-xs text-muted-foreground">旧绑定对象和字段独立生效。新读取范围不会自动授权旧绑定，也不会替代其字段限制。</p>
      {(["resources", "channels", "spatial", "artifacts"] as const).map(
        (kind) =>
          kind === "channels" ? (
            <BindingEditor
              key={kind}
              label="通道范围"
              value={{
                kind,
                ids: grant.scope.channels,
                dynamic: grant.scope.dynamic,
                from: grant.scope.from,
                to: grant.scope.to,
              }}
              onChange={(b) =>
                update({
                  scope: {
                    ...grant.scope,
                    channels: b.ids,
                    dynamic: b.dynamic,
                    from: b.from,
                    to: b.to,
                  },
                })
              }
            />
          ) : (
            <ObjectPicker
              key={kind}
              label={
                {
                  resources: "资源范围",
                  spatial: "空间固定版本",
                  artifacts: "分析成果固定版本",
                }[kind]
              }
              kind={kind}
              ids={grant.scope[kind]}
              onChange={(ids) =>
                update({ scope: { ...grant.scope, [kind]: ids } })
              }
            />
          ),
      )}
      <fieldset className="flex flex-wrap gap-4">
        <legend className="mb-2 text-sm font-medium">允许字段</legend>
        {["metadata", "values", "quality", "source", "properties"].map((f) => (
          <label key={f} className="flex gap-2 text-sm">
            <input
              type="checkbox"
              checked={grant.fields.includes(f)}
              onChange={(e) =>
                update({
                  fields: e.target.checked
                    ? [...grant.fields, f]
                    : grant.fields.filter((v) => v !== f),
                })
              }
            />
            {f}
          </label>
        ))}
      </fieldset>
      </details>
      <label className="block text-sm">
        有效期截止（UTC）
        <Input
          value={grant.valid_until}
          onChange={(e) => update({ valid_until: e.target.value })}
        />
      </label>
      <div className="flex gap-2">
        <Button
          disabled={disabled}
          onClick={() =>
            void save({ ...grant, revoked: false }).catch(() => {})
          }
        >
          {archived ? "保存收窄授权" : "保存读取授权"}
        </Button>
        <Button
          variant="outline"
          disabled={disabled || !data.grant || data.grant.revoked}
          onClick={() => void save({ ...grant, revoked: true }).catch(() => {})}
        >
          撤销读取授权
        </Button>
      </div>
    </fieldset>
  );
}

export function InstanceInfo({instance,disabled,save}:{instance:DisplayDetail['instance'];disabled:boolean;save:(value:object)=>Promise<unknown>}) {
  const project=useProject(),{client}=useIdentity();
  const [name,setName]=useState(instance.name),[restricted,setRestricted]=useState(instance.viewers.length>0),[viewers,setViewers]=useState(instance.viewers),[search,setSearch]=useState('');
  const members=useInfiniteQuery({queryKey:[...project.key,'display-viewers',search],initialPageParam:'',queryFn:({pageParam,signal})=>client.projectMembers(project.project.tenant_id,project.project.id,{q:search,status:'active',cursor:pageParam},signal),getNextPageParam:p=>p.next_cursor||undefined,enabled:project.available&&restricted&&project.can(P.ProjectMemberRead)});
  return <form className="space-y-4" onSubmit={e=>{e.preventDefault();void save({name,state:instance.state,viewers:restricted?viewers:[]}).catch(()=>{});}}>
    <label className="block text-sm">实例名称<Input value={name} required maxLength={100} disabled={disabled} onChange={e=>setName(e.target.value)}/></label>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={restricted} disabled={disabled||!project.can(P.ProjectMemberRead)} onChange={e=>setRestricted(e.target.checked)}/>限制为指定项目成员</label>
    <p className="text-xs text-muted-foreground">默认允许有大屏查看资格的项目成员。指定成员仍需具备当前业务读取权限；实例管理者保留管理访问。</p>
    {restricted&&<fieldset disabled={disabled} className="space-y-2"><legend className="text-sm">允许查看的成员（已选 {viewers.length} 人）</legend><Input aria-label="搜索大屏查看成员" value={search} onChange={e=>setSearch(e.target.value)} placeholder="姓名或用户名"/>
      {members.error&&<p role="alert" className="text-sm text-destructive">{members.error.message}</p>}
      <div className="max-h-56 overflow-y-auto">{members.data?.pages.flatMap(p=>p.items).map(m=><label key={m.id} className="flex min-h-9 items-center gap-2 text-sm"><input type="checkbox" checked={viewers.includes(m.account_id)} onChange={e=>setViewers(e.target.checked?[...viewers,m.account_id]:viewers.filter(id=>id!==m.account_id))}/>{m.display_name} · {m.username}</label>)}</div>
      {members.hasNextPage&&<Button type="button" variant="outline" size="sm" disabled={members.isFetchingNextPage} onClick={()=>void members.fetchNextPage()}>更多成员</Button>}
    </fieldset>}
    <Button size="sm" disabled={disabled||!name.trim()||(restricted&&!viewers.length)}>保存实例信息</Button>
  </form>;
}
