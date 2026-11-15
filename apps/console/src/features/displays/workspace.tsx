import { ObjectHistory } from '../audit/object-history';
import { DomainFiles } from '../files/domain-files';
import { displayAdapter } from '../files/adapters/display';
import { transferFile, type UploadProgress as FileProgress } from '../files/upload-controller';
import { UploadPicker, UploadProgress } from '@infrapro/ui/patterns/files';
import { phases } from '../files/state';
import { TaskLink } from '../tasks/common';
import { useEffect, useRef, useState } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Permissions as P,
  ApiError,
  type DisplayBinding,
  type DisplayConfig,
  type DisplayDetail,
  type DisplayGrant,
  type DisplayList,
  type DisplayPackage,
} from "@infrapro/api-client";
import { useIdentity } from "@infrapro/api-client/react";
import { Button } from "@infrapro/ui/components/button";
import { Input } from "@infrapro/ui/components/input";
import { Badge } from "@infrapro/ui/components/badge";
import { useProject } from "../projects/context";
import {
  BindingEditor,
  SchemaField,
  fieldClass,
  ObjectPicker,
} from "./forms";
import { requestUUID as uuid } from "./crypto";
import { upgradeConfig } from "./config-upgrade";

const errorMessage=(error:unknown)=>error instanceof ApiError && error.issues.length ? `${error.message} ${error.issues.map(i=>`${i.path}：${i.message}`).join('；')}` : error instanceof Error? error.message:'操作失败';
const names: Record<string, string> = {
  enabled: "已启用",
  disabled: "已停用",
  archived: "已归档",
  receiving: "接收中",
  uploaded: "已提交",
  queued: "待处理",
  validating: "校验中",
  ready: "已就绪",
  failed: "失败",
  canceled: "已取消",
  starting: "启动中",
  degraded: "异常",
  expired: "已过期",
  revoked: "已撤销",
  active: "有效",
  unpaired: "待配对",
  closed: "已关闭",
  missing: "文件缺失",
  reclaiming: "待回收",
};
export function DisplaysWorkspace({ instanceID, packageID }: { instanceID?: string; packageID?: string }) {
  const project = useProject(),
    { client } = useIdentity(),
    cache = useQueryClient(),
    scope = {
      tenantId: project.project.tenant_id,
      projectId: project.project.id,
    },
    key = [...project.key, "displays"];
  const [selected, setSelected] = useState(instanceID ?? ""),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const state = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => client.displayList(scope, signal),
    enabled: project.available && project.can(P.ProjectDisplayView),
    refetchInterval: 15000,
    retry: false,
  });
  const active = project.project.status === "active";
  async function command(path: string, body: object, method = "POST") {
    setBusy(true);
    setError("");
    try {
      const result = await client.displayCommand(
        scope,
        path,
        { request_id: uuid(), reason: "大屏管理", ...body },
        method,
      );
      setName("");
      await cache.invalidateQueries({ queryKey: key });
      if (path === "/instances") setSelected(result.id);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  if (!project.can(P.ProjectDisplayView))
    return <p>您没有项目大屏查看权限。</p>;
  return (
    <section className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">项目大屏</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            管理独立程序、读取范围与发布，查看运行和终端状态。
          </p>
        </div>
        {state.data && project.can(P.ProjectDisplaySettings) && (
          <Button
            variant="outline"
            disabled={busy || (!active && !state.data.settings.enabled)}
            onClick={() =>
              void command(
                "/settings",
                {
                  expected_revision: state.data!.settings.revision,
                  enabled: !state.data!.settings.enabled,
                },
                "PUT",
              )
            }
          >
            {state.data.settings.enabled ? "停用项目大屏" : "启用项目大屏"}
          </Button>
        )}
      </header>
      {(error || state.error) && (
        <p
          role="alert"
          className="rounded-lg border border-destructive p-3 text-sm text-destructive"
        >
          {error || state.error?.message}
        </p>
      )}
      {!state.data ? (
        <p role="status">正在读取大屏…</p>
      ) : !state.data.settings.enabled ? (
        <div className="rounded-xl border border-dashed p-12 text-center">
          <h2 className="font-medium">项目大屏尚未启用</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            启用后可创建多个独立实例。项目数据权限单独配置。
          </p>
        </div>
      ) : (
        <div className="grid gap-5 lg:grid-cols-[260px_minmax(0,1fr)]">
          <aside className="space-y-3">
            <div className="space-y-2 rounded-xl border p-3">
              {state.data.items.length === 0 && (
                <p className="p-3 text-sm text-muted-foreground">
                  还没有大屏实例
                </p>
              )}
              {state.data.items.map((item) => (
                <button
                  key={item.id}
                  className={`flex min-h-14 w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm ${selected === item.id ? "bg-primary/10 text-primary" : "hover:bg-muted"}`}
                  onClick={() => setSelected(item.id)}
                >
                  <span className="break-words font-medium">{item.name}</span>
                  <Badge variant="outline">{names[item.state]}</Badge>
                </button>
              ))}
            </div>
            {project.can(P.ProjectDisplayManage) && active && (
              <form
                className="space-y-2 rounded-xl border p-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  void command("/instances", { name, expected_revision: "0" });
                }}
              >
                <label htmlFor="display-name" className="text-sm font-medium">
                  新建大屏实例
                </label>
                <Input
                  id="display-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  maxLength={100}
                  required
                  placeholder="例如：项目综合监测"
                />
                <Button
                  type="submit"
                  disabled={busy || !name.trim()}
                  className="w-full"
                >
                  创建实例
                </Button>
              </form>
            )}
          </aside>
          {selected ? (
            <InstancePanel key={`${selected}:${packageID??""}`} id={selected} list={state.data} packageID={packageID} />
          ) : (
            <div className="rounded-xl border p-12 text-center text-sm text-muted-foreground">
              选择或创建一个实例，开始配置
            </div>
          )}
        </div>
      )}
    </section>
  );
}
function InstancePanel({ id, list, packageID }: { id: string; list: DisplayList; packageID?: string }) {
  const project = useProject(),
    { client } = useIdentity(),
    cache = useQueryClient(),
    scope = {
      tenantId: project.project.tenant_id,
      projectId: project.project.id,
    },
    key = [...project.key, "displays", id];
  const state = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => client.displayDetail(scope, id, signal),
    enabled: project.available,
    refetchInterval: 5000,
    retry: false,
  });
  const [tab, setTab] = useState("程序"),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState(""),
    [error, setError] = useState("");
  const [terminalName, setTerminalName] = useState(""),
    [preparation, setPreparation] = useState<{
      hash: string;
      binding_sets: Record<string, string[]>;
      grant_revision: string;
    } | null>(null);
  const uploadAbort = useRef<AbortController | null>(null), uploadAdapter=useRef<ReturnType<typeof displayAdapter>|null>(null), [fileProgress,setFileProgress]=useState<FileProgress|null>(null);
  useEffect(()=>()=>uploadAbort.current?.abort(),[]);
  const data = state.data,
    instance = data?.instance,
    root = `/instances/${id}`;
  const editable =
    project.project.status === "active" && instance?.state !== "archived";
  async function command(
    path: string,
    body: object = {},
    method = "POST",
    revision?: string,
  ) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await client.displayCommand(
        scope,
        root + path,
        {
          request_id: uuid(),
          reason: "大屏管理",
          expected_revision: revision ?? state.data!.instance.revision,
          ...body,
        },
        method,
      );
      setPreparation(null);
      await cache.invalidateQueries({ queryKey: [...project.key, "displays"] });
      if (result.pair_code)
        setNotice(
          `一次性配对码：${result.pair_code}（10 分钟有效，仅显示一次）`,
        );
      else setNotice("已保存");
      return result;
    } catch (e) {
      setError(errorMessage(e));
      throw e;
    } finally {
      setBusy(false);
    }
  }
  async function upload(file: File) {
    if (file.size > 128 * 1024 ** 2) {
      setError("ZIP 最大 128 MiB");
      return;
    }
    uploadAbort.current?.abort();
    const controller = new AbortController();
    uploadAbort.current = controller;
    setBusy(true);
    setError("");
    try {
      uploadAdapter.current ??= displayAdapter(client,scope,id,state.data!.instance.revision);
      await transferFile(uploadAdapter.current,file,controller.signal,setFileProgress);
      controller.signal.throwIfAborted();uploadAdapter.current=null;
      await cache.invalidateQueries({queryKey:[...project.key,'displays']});
      setNotice("上传完成，等待独立校验。");
    } catch (e) {
      setError(e instanceof Error ? e.message : "上传失败，可取消或重新提交");
    } finally {
      setBusy(false);
    }
  }
  const run = (promise: Promise<unknown>) => void promise.catch(() => {});
  if (state.error)
    return (
      <p role="alert" className="text-destructive">
        {state.error.message}
      </p>
    );
  if (!data || !instance) return <p role="status">正在读取实例…</p>;
  return (
    <article className="min-w-0 space-y-4 rounded-xl border p-5">
      <header className="space-y-3">
        <div className="flex flex-wrap justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold">{instance.name}</h2><ObjectHistory objectType="display_instance" objectId={id}/>
            <p className="mt-1 text-xs text-muted-foreground">
              修订 {instance.revision} · {names[instance.state]}
              {instance.current_release_id ? " · 已发布" : " · 尚未发布"}
            </p>
          </div>
          <a
            className="text-sm text-primary underline"
            href={data.access_url}
            target="_blank"
            rel="noreferrer"
          >
            打开正式入口
          </a>
        </div>
        {project.can(P.ProjectDisplayManage) && (
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={busy || !editable}
              onClick={() =>
                run(
                  command(
                    "",
                    {
                      name: instance.name,
                      state:
                        instance.state === "enabled" ? "disabled" : "enabled",
                      viewers: instance.viewers,
                    },
                    "PATCH",
                  ),
                )
              }
            >
              {instance.state === "enabled" ? "停用实例" : "启用实例"}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={busy || instance.state === "archived"}
              onClick={() =>
                run(
                  command(
                    "",
                    {
                      name: instance.name,
                      state: "archived",
                      viewers: instance.viewers,
                    },
                    "PATCH",
                  ),
                )
              }
            >
              归档实例
            </Button>
          </div>
        )}
      </header>
      {project.can(P.ProjectDisplayManage) && <InstanceInfo key={instance.revision} instance={instance} disabled={busy||!editable} save={body=>command('',body,'PATCH')} />}
      <nav
        className="flex flex-wrap gap-1 border-b pb-3"
        aria-label="大屏管理分区"
      >
        {["程序", "配置", "读取授权", "发布", "运行", "终端", "记录"].map(
          (t) => (
            <Button
              size="sm"
              variant={tab === t ? "secondary" : "ghost"}
              key={t}
              onClick={() => setTab(t)}
              aria-pressed={tab === t}
            >
              {t}
            </Button>
          ),
        )}
      </nav>
      {error && (
        <p
          role="alert"
          className="rounded border border-destructive p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      {notice && (
        <p
          role="status"
          className="break-all rounded border bg-muted p-3 text-sm"
        >
          {notice}
        </p>
      )}
      {tab === "程序" && (
        <div className="space-y-4">
          {project.can(P.ProjectDisplayUpload) && editable && (
            <div className="block rounded-lg border border-dashed p-5 text-sm">
              <span className="mb-2 block font-medium">
                上传构建 ZIP · 最大 128 MiB
              </span>
              <UploadPicker label="上传大屏 ZIP" accept=".zip" disabled={busy} hint="ZIP 最大 128 MiB；保留原包校验与隔离运行流程。" onChange={file=>{if(file)void upload(file);}}/>
              {fileProgress&&<UploadProgress phase={phases[fileProgress.phase]} bytes={fileProgress.bytes} total={fileProgress.total} busy={busy} onPause={()=>uploadAbort.current?.abort()}/>}
              {busy && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => uploadAbort.current?.abort()}
                >
                  中止传输
                </Button>
              )}
            </div>
          )}
          {data.uploads?.map((u) => (
            <div key={u.id} className="space-y-2 rounded-lg border p-3 text-sm">
              <p>
                {names[u.job?.state ?? u.state] ?? u.state} · {u.size} 字节
              </p>
              {u.job?.issues?.map((i, n) => (
                <p key={n} className="text-destructive">
                  {i.path}：{i.message}
                </p>
              ))}
              <div className="flex gap-2">
                {u.job ? <TaskLink source={{source_module:'display',source_kind:'job',source_root_id:u.job.id}} /> : project.can(P.ProjectDisplayUpload) && u.state !== 'canceled' && <Button className="min-h-11" variant="outline" disabled={busy} onClick={() => run(command(`/uploads/${u.id}/cancel`))}>取消上传</Button>}
              </div>
            </div>
          ))}
          {data.packages?.map((p) => (
            <div key={p.id} id={`display-package-${p.id}`} tabIndex={-1} ref={element=>{if(element&&packageID===p.id){element.scrollIntoView({block:"center"});element.focus({preventScroll:true});}}} className={`space-y-2 rounded-lg border p-4 ${packageID===p.id?"ring-2 ring-ring":""}`}>
              <div className="flex justify-between gap-2">
                <strong>
                  {p.manifest.name ?? p.manifest.app_key} · {p.manifest.version}
                </strong>
                <Badge variant="outline">{names[p.state]}</Badge>
              </div>
              <ObjectHistory objectType="display_package" objectId={id} filter={{reference_id:p.id}} label="包管理历史"/><DomainFiles files={[{ref:{kind:'display.package',scope:{kind:'project',tenantId:scope.tenantId,projectId:scope.projectId},instanceId:id,packageId:p.id},display_name:`${p.manifest.name??p.manifest.app_key} ${p.manifest.version}`,media_type:'application/zip',sha256:p.sha256,state:p.state==='validated'?'available':'preparing',allowed_actions:[]}]} actions={()=>null}/>
              <p className="text-sm">
                {p.manifest.files.length} 个文件 · 必需能力{" "}
                {Object.keys(p.manifest.required_capabilities).join("、") ||
                  "无"}
              </p>
              {project.can(P.ProjectDisplayUpload) && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy || !editable}
                  onClick={() =>
                    run(command(`/packages/${p.id}`, {}, "DELETE"))
                  }
                >
                  回收无引用包
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
      {tab === "配置" && (
        <Configuration
          data={data}
          busy={busy || !editable || !project.can(P.ProjectDisplayConfigure)}
          save={(body) => command("/configs", body)}
          runtime={list.runtime_origin}
        />
      )}
      {tab === "读取授权" && (
        <GrantForm
          key={data.grant?.revision ?? "new"}
          data={data}
          capabilities={list.capabilities}
          disabled={busy || !project.can(P.ProjectDisplayGrant)}
          archived={!editable}
          save={(grant) => command("/grant", { grant }, "PUT")}
        />
      )}
      {tab === "发布" && (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            发布准备会复验包、配置、绑定和当前授权。确认后按准备结果与修订执行发布。
          </p>
          {data.instance.draft_config_id &&
            project.can(P.ProjectDisplayPublish) && (
              <Button
                disabled={busy || !editable}
                onClick={() => {
                  setError("");
                  void client
                    .displayPrepare(
                      scope,
                      id,
                      data.instance.draft_config_id,
                      instance.revision,
                    )
                    .then(setPreparation)
                    .catch((e) => setError(errorMessage(e)));
                }}
              >
                核验当前草稿
              </Button>
            )}
          {preparation && (
            <div className="space-y-3 rounded-lg border bg-muted p-4">
              <p className="font-medium">核验通过</p>
              <p className="text-sm">
                读取授权修订 {preparation.grant_revision} ·{" "}
                {Object.entries(preparation.binding_sets)
                  .map(([k, ids]) => `${k}: ${ids.length} 个对象`)
                  .join("；")}
              </p>
              <Button
                disabled={busy || !editable}
                onClick={() =>
                  run(
                    command("/publish", {
                      config_id: instance.draft_config_id,
                      preparation: preparation.hash,
                    }),
                  )
                }
              >
                发布此修订
              </Button>
            </div>
          )}
          {data.releases?.map((r) => (
            <div className="space-y-2 rounded-lg border p-4" key={r.id}>
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">
                  {r.withdrawn
                    ? "已撤回"
                    : instance.current_release_id === r.id
                      ? "当前发布"
                      : "历史发布"}
                </Badge>
                <span className="text-sm">
                  {new Date(r.created_at).toLocaleString()}
                </span>
              </div>
              <p className="break-all text-xs text-muted-foreground">
                发布 {r.id}
              </p>
              {project.can(P.ProjectDisplayPublish) && (
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={
                      busy ||
                      !editable ||
                      r.withdrawn ||
                      instance.current_release_id === r.id
                    }
                    onClick={() => {
                      void client
                        .displayPrepare(
                          scope,
                          id,
                          r.config_id,
                          instance.revision,
                        )
                        .then((prep) =>
                          command("/rollback", {
                            release_id: r.id,
                            preparation: prep.hash,
                          }),
                        )
                        .catch((e) => setError(e.message));
                    }}
                  >
                    按当前授权回退至此版本
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy || r.withdrawn}
                    onClick={() =>
                      run(command("/withdraw", { release_id: r.id }))
                    }
                  >
                    撤回发布
                  </Button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
      {tab === "运行" && (
        <div className="space-y-3">
          <p className="text-sm text-muted-foreground">
            目标发布：{instance.current_release_id || "无"}
            。连接与应用就绪分别记录。
          </p>
          {data.sessions?.map((s) => (
            <div key={s.id} className="rounded-lg border p-3 text-sm">
              <p>
                {s.preview ? "草稿预览" : "正式运行"} · {names[s.state]} ·
                最近心跳 {new Date(s.last_seen_at).toLocaleString()}
              </p>
              <p className="break-all text-xs text-muted-foreground">
                实际发布 {s.release_id || "草稿"} · 会话 {s.id}
              </p>
              <p className="text-xs text-muted-foreground">最近有效读取 {s.last_data_at ? new Date(s.last_data_at).toLocaleString() : "尚无"} · 查询状态 {s.last_query_state || "尚无"}</p>
              {s.last_error && (
                <p className="text-destructive">{s.last_error}</p>
              )}
            </div>
          ))}
        </div>
      )}
      {tab === "终端" && (
        <div className="space-y-4">
          {project.can(P.ProjectDisplayTerminals) && editable && (
            <form
              className="flex flex-wrap gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                run(
                  command("/terminals", {
                    name: terminalName,
                    valid_until: new Date(
                      Date.now() + 90 * 86400000,
                    ).toISOString(),
                  }),
                );
              }}
            >
              <Input
                aria-label="终端名称"
                placeholder="终端名称"
                value={terminalName}
                onChange={(e) => setTerminalName(e.target.value)}
                required
              />
              <Button disabled={busy}>签发配对码（90 天）</Button>
            </form>
          )}
          <p className="text-sm text-muted-foreground">
            在固定终端打开{" "}
            <a
              className="text-primary underline"
              href={`${list.runtime_origin}/terminal`}
              target="_blank"
              rel="noreferrer"
            >
              配对入口
            </a>
            ，输入一次性配对码。
          </p>
          {data.terminals?.map((t) => (
            <div className="space-y-2 rounded-lg border p-4" key={t.id}>
              <p className="font-medium">
                {t.name} · {names[t.state]}
              </p>
              <p className="text-sm">
                到期 {new Date(t.valid_until).toLocaleDateString()} · 代次{" "}
                {t.generation}
              </p>
              <p className="break-all text-xs text-muted-foreground">
                实际发布 {t.actual_release_id || "未就绪"}
              </p>
              {t.last_error && (
                <p className="text-sm text-destructive">{t.last_error}</p>
              )}
              {project.can(P.ProjectDisplayTerminals) && (
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy || !editable}
                    onClick={() =>
                      run(
                        command(
                          `/terminals/${t.id}`,
                          { state: "reset" },
                          "PATCH",
                        ),
                      )
                    }
                  >
                    重置配对
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy || t.state === "revoked"}
                    onClick={() =>
                      run(
                        command(
                          `/terminals/${t.id}`,
                          { state: "revoked" },
                          "PATCH",
                        ),
                      )
                    }
                  >
                    撤销
                  </Button>
                  <label className="text-sm">
                    改绑实例
                    <select
                      aria-label={`改绑 ${t.name}`}
                      className={fieldClass}
                      value=""
                      disabled={busy || !editable}
                      onChange={(e) => {
                        if (e.target.value)
                          run(
                            command(
                              `/terminals/${t.id}`,
                              { state: "assign", instance_id: e.target.value },
                              "PATCH",
                            ),
                          );
                      }}
                    >
                      <option value="">请选择</option>
                      {list.items
                        .filter((i) => i.id !== id && i.state === "enabled")
                        .map((i) => (
                          <option key={i.id} value={i.id}>
                            {i.name}
                          </option>
                        ))}
                    </select>
                  </label>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
      {tab === "记录" && (
        <ol className="space-y-3">
          {data.events?.map((e) => (
            <li className="rounded-lg border p-3 text-sm" key={e.id}>
              <p>
                {e.kind} · {new Date(e.created_at).toLocaleString()}
              </p>
              <details>
                <summary className="cursor-pointer text-muted-foreground">
                  详情
                </summary>
                <pre className="max-h-52 overflow-auto whitespace-pre-wrap break-all text-xs">
                  {JSON.stringify(e.details, null, 2)}
                </pre>
              </details>
            </li>
          ))}
        </ol>
      )}
    </article>
  );
}
function Configuration({
  data,
  busy,
  save,
  runtime,
}: {
  data: DisplayDetail;
  busy: boolean;
  save: (body: object) => Promise<unknown>;
  runtime: string;
}) {
  const project = useProject();
  const [selected, setSelected] = useState(""),
    [parameters, setParameters] = useState<Record<string, unknown>>({}),
    [bindings, setBindings] = useState<Record<string, DisplayBinding>>({}),
    [mode, setMode] = useState<DisplayConfig["mode"]>("fixed"),
    [upgrade, setUpgrade] = useState<ReturnType<typeof upgradeConfig>>();
  const p = data.packages?.find((p) => p.id === selected);
  function choose(p: DisplayPackage, c?: DisplayConfig) {
    const prior = c ?? (selected ? {parameters, bindings, mode} : data.configs?.find(c => c.id === data.instance.draft_config_id));
    const result = upgradeConfig(p, prior);
    setSelected(p.id);
    setParameters(result.parameters);
    setBindings(result.bindings);
    setMode(result.mode);
    setUpgrade(prior ? result : undefined);
  }
  return (
    <div className="space-y-5">
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
              void save({ package_id: p.id, parameters, bindings, mode }).catch(
                () => {},
              )
            }
          >
            保存不可变草稿修订
          </Button>
        </>
      )}
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
function GrantForm({
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
    <div className="space-y-5">
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
    </div>
  );
}

function InstanceInfo({instance,disabled,save}:{instance:DisplayDetail['instance'];disabled:boolean;save:(value:object)=>Promise<unknown>}) {
  const project=useProject(),{client}=useIdentity();
  const [open,setOpen]=useState(false),[name,setName]=useState(instance.name),[restricted,setRestricted]=useState(instance.viewers.length>0),[viewers,setViewers]=useState(instance.viewers),[search,setSearch]=useState('');
  const members=useInfiniteQuery({queryKey:[...project.key,'display-viewers',search],initialPageParam:'',queryFn:({pageParam,signal})=>client.projectMembers(project.project.tenant_id,project.project.id,{q:search,status:'active',cursor:pageParam},signal),getNextPageParam:p=>p.next_cursor||undefined,enabled:open&&restricted&&project.can(P.ProjectMemberRead)});
  return <details className="rounded-lg border p-3" onToggle={e=>setOpen(e.currentTarget.open)}><summary className="cursor-pointer text-sm font-medium">实例名称与人员范围</summary><form className="mt-4 space-y-4" onSubmit={e=>{e.preventDefault();void save({name,state:instance.state,viewers:restricted?viewers:[]}).catch(()=>{});}}>
    <label className="block text-sm">实例名称<Input value={name} required maxLength={100} disabled={disabled} onChange={e=>setName(e.target.value)}/></label>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={restricted} disabled={disabled||!project.can(P.ProjectMemberRead)} onChange={e=>setRestricted(e.target.checked)}/>限制为指定项目成员</label>
    <p className="text-xs text-muted-foreground">默认允许有大屏查看资格的项目成员。指定成员仍需具备当前业务读取权限；实例管理者保留管理访问。</p>
    {restricted&&<fieldset disabled={disabled} className="space-y-2"><legend className="text-sm">允许查看的成员（已选 {viewers.length} 人）</legend><Input aria-label="搜索大屏查看成员" value={search} onChange={e=>setSearch(e.target.value)} placeholder="姓名或用户名"/>
      {members.error&&<p role="alert" className="text-sm text-destructive">{members.error.message}</p>}
      <div className="max-h-56 overflow-y-auto">{members.data?.pages.flatMap(p=>p.items).map(m=><label key={m.id} className="flex min-h-9 items-center gap-2 text-sm"><input type="checkbox" checked={viewers.includes(m.account_id)} onChange={e=>setViewers(e.target.checked?[...viewers,m.account_id]:viewers.filter(id=>id!==m.account_id))}/>{m.display_name} · {m.username}</label>)}</div>
      {members.hasNextPage&&<Button type="button" variant="outline" size="sm" disabled={members.isFetchingNextPage} onClick={()=>void members.fetchNextPage()}>更多成员</Button>}
    </fieldset>}
    <Button size="sm" disabled={disabled||!name.trim()||(restricted&&!viewers.length)}>保存实例信息</Button>
  </form></details>;
}
