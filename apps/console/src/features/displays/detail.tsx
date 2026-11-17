import { ObjectHistory } from '../audit/object-history';
import { DomainFiles } from '../files/domain-files';
import { displayAdapter } from '../files/adapters/display';
import { transferFile, type UploadProgress as FileProgress } from '../files/upload-controller';
import { UploadPicker, UploadProgress } from '@infrapro/ui/patterns/files';
import { phases } from '../files/state';
import { TaskLink } from '../tasks/common';
import { useEffect, useRef, useState, type RefObject } from "react";
import { ArrowLeft, ArrowUpRight, Settings2 } from "lucide-react";
import { ResponsivePanel } from "@infrapro/ui/patterns/responsive-panel";
import { EmptyState, ErrorState, LoadingState } from "@infrapro/ui/patterns/feedback";
import { DisplayOverview } from "./overview";
import { Publication } from "./publication";
import { Configuration, GrantForm, InstanceInfo } from "./editors";
import { ConfirmDisplayAction } from "./confirm-action";
import { displaySummary, type DisplayView } from "./presentation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Permissions as P,
  ApiError,
  type DisplayList,
} from "@infrapro/api-client";
import { useIdentity } from "@infrapro/api-client/react";
import { Button } from "@infrapro/ui/components/button";
import { Input } from "@infrapro/ui/components/input";
import { Badge } from "@infrapro/ui/components/badge";
import { useProject } from "../projects/context";
import { fieldClass } from "./forms";
import { requestUUID as uuid } from "./crypto";

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
export function InstancePanel({ id, list, packageID, initialView, close, returnFocus }: { id: string; list: DisplayList; packageID?: string; initialView: DisplayView; close: () => void; returnFocus: RefObject<HTMLElement | null> }) {
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
    enabled: project.available && project.can(P.ProjectDisplayView),
    refetchInterval: 5000,
    staleTime: 0,
    refetchOnWindowFocus: 'always',
    retry: false,
  });
  const [tab, setTab] = useState<DisplayView>(packageID ? "程序版本" : initialView),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState(""),
    [error, setError] = useState("");
  const [terminalName, setTerminalName] = useState("");
  const [editingPackage, setEditingPackage] = useState<string>();
  const focusedPackage = useRef(false);
  const uploadAbort = useRef<AbortController | null>(null), uploadAdapter=useRef<ReturnType<typeof displayAdapter>|null>(null), [fileProgress,setFileProgress]=useState<FileProgress|null>(null);
  useEffect(()=>()=>uploadAbort.current?.abort(),[]);
  const data = state.data,
    instance = data?.instance,
    root = `/instances/${id}`;
  const editable =
    project.available && project.project.status === "active" && instance?.state !== "archived" && list.settings.enabled;
  async function command(
    path: string,
    body: object = {},
    method = "POST",
    revision?: string,
  ) {
    if (!project.available) throw new Error("正在核验项目身份，请稍后重试。");
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
      await cache.invalidateQueries({ queryKey: [...project.key, "displays"] });
      if (result.pair_code)
        setNotice(
          `一次性配对码：${result.pair_code}（10 分钟有效，仅显示一次）`,
        );
      else setNotice("已保存");
      if (path === "/terminals") setTerminalName("");
      return result;
    } catch (e) {
      setError(errorMessage(e));
      throw e;
    } finally {
      setBusy(false);
    }
  }
  async function upload(file: File) {
    if (!project.available) return;
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
  const summary = data && displaySummary(data, list.settings.enabled);
  const primaryViews: DisplayView[] = ["概览", "程序版本", "发布记录", "运行与终端", "记录"];
  const management = summary?.management;
  const subview = !primaryViews.includes(tab);
  const go = (view: DisplayView) => { setTab(view); setEditingPackage(undefined); setError(""); setNotice(""); };
  return <ResponsivePanel open onOpenChange={open => { if (!open) close(); }} title={instance?.name ?? '大屏详情'} description={instance ? `${summary?.status ?? ''} · 修订 ${instance.revision}` : '正在读取大屏'} size="wide" returnFocus={returnFocus} fallbackFocusSelector="#display-search" footer={data && !state.error && <div className="flex flex-wrap items-center gap-2">
    {project.can(P.ProjectDisplayManage) && <Button variant="ghost" size="icon" aria-label="大屏设置" onClick={() => go('设置')}><Settings2 aria-hidden="true" /></Button>}
    <Badge variant="outline">{summary?.status}</Badge>
    <div className="ml-auto">{project.available && summary?.canOpen ? <Button asChild><a href={data.access_url} target="_blank" rel="noreferrer">打开大屏<ArrowUpRight aria-hidden="true" /></a></Button> : <Button disabled>打开大屏<ArrowUpRight aria-hidden="true" /></Button>}</div>
  </div>}>
    {state.error ? <ErrorState message={state.error.message} onRetry={() => void state.refetch()} /> : !data || !instance ? <LoadingState label="正在读取大屏…" /> : <article className="min-w-0 space-y-5">
      {management && (subview ? <div className="flex flex-wrap items-center gap-3 border-b pb-3"><Button size="sm" variant="ghost" onClick={() => go('概览')}><ArrowLeft aria-hidden="true" />返回概览</Button><h3 className="text-sm font-semibold">{tab === '配置' ? '工作草稿' : tab === '设置' ? '大屏设置' : tab === '读取授权' ? '数据授权' : '发布更改'}</h3></div> : <nav className="flex flex-wrap gap-1 border-b pb-3" aria-label="大屏管理分区">{primaryViews.map(t => <Button size="sm" variant={tab === t ? 'secondary' : 'ghost'} key={t} onClick={() => go(t)} aria-pressed={tab === t}>{t}</Button>)}</nav>)}
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
      {(tab === '概览' || !management) && <DisplayOverview data={data} list={list} editable={editable} busy={busy} go={go} />}
      {management && tab === '设置' && <div className="space-y-6">
        {project.can(P.ProjectDisplayManage) && <InstanceInfo key={instance.revision} instance={instance} disabled={!project.available || busy || !editable} save={body => command('', body, 'PATCH')} />}
        <div className="flex flex-wrap gap-2 border-t pt-5">
          {project.can(P.ProjectDisplayManage) && <>
            <ConfirmDisplayAction label={instance.state === 'enabled' ? '停用大屏' : '启用大屏'} description={instance.state === 'enabled' ? '停用后，访问入口与固定终端将停止运行。程序和配置会保留。' : '恢复此大屏的访问与运行。'} disabled={!project.available || busy || instance.state === 'archived' || (instance.state !== 'enabled' && !editable)} action={() => command('', { name: instance.name, state: instance.state === 'enabled' ? 'disabled' : 'enabled', viewers: instance.viewers }, 'PATCH')} />
            <ConfirmDisplayAction label="归档大屏" description="归档后将停止运行，并保留程序、配置与记录供查询。归档无法恢复。" disabled={!project.available || busy || instance.state === 'archived'} action={() => command('', { name: instance.name, state: 'archived', viewers: instance.viewers }, 'PATCH')} />
          </>}
        </div>
      </div>}
      {management && tab === "程序版本" && (
        <div className="space-y-4">
          {project.can(P.ProjectDisplayUpload) && editable && (
            <div className="block rounded-lg border border-dashed p-5 text-sm">
              <span className="mb-2 block font-medium">
                上传构建 ZIP · 最大 128 MiB
              </span>
              <UploadPicker label="上传大屏 ZIP" accept=".zip" disabled={!project.available || busy} hint="构建 ZIP，最大 128 MiB" onChange={file=>{if(file)void upload(file);}}/>
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
          {!!data.uploads?.length && <details className="rounded-lg border px-4 py-3"><summary className="cursor-pointer text-sm font-medium">上传任务 · {data.uploads.length}</summary><div className="mt-3 space-y-2">{data.uploads.map((u) => (
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
                {u.job?.id ? <TaskLink source={{source_module:'display',source_kind:'job',source_root_id:u.job.id}} /> : project.can(P.ProjectDisplayUpload) && u.state !== 'canceled' && <Button className="min-h-11" variant="outline" disabled={!project.available || busy} onClick={() => run(command(`/uploads/${u.id}/cancel`))}>取消上传</Button>}
              </div>
            </div>
          ))}
          </div></details>}
          {!data.packages?.length && <EmptyState title="暂无程序版本" />}
          {data.packages?.map((p) => (
            <div key={p.id} id={`display-package-${p.id}`} tabIndex={-1} ref={element=>{if(element&&packageID===p.id&&!focusedPackage.current){focusedPackage.current=true;element.scrollIntoView({block:"center"});element.focus({preventScroll:true});}}} className={`space-y-2 rounded-lg border p-4 ${packageID===p.id?"ring-2 ring-ring":""}`}>
              <div className="flex justify-between gap-2">
                <strong>
                  {p.manifest.name ?? p.manifest.app_key} · {p.manifest.version}
                </strong>
                <Badge variant="outline">{names[p.state]}</Badge>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {summary?.livePackage?.id === p.id && <Badge variant="secondary">线上版本</Badge>}
                {summary?.draftPackage?.id === p.id && <Badge variant="outline">工作草稿</Badge>}
                {project.can(P.ProjectDisplayConfigure) && p.state === 'ready' && <Button size="sm" variant="outline" disabled={!project.available || busy || !editable} onClick={() => { setEditingPackage(p.id); setTab('配置'); }}>用于草稿</Button>}
              </div>
              {!!Object.keys(p.manifest.read_view_schema??{}).length&&<div className="space-y-2 rounded-lg border p-3"><h4 className="text-sm font-medium">程序声明的读取视图</h4>{Object.entries(p.manifest.read_view_schema??{}).map(([name,v])=><p key={name} className="text-xs">{name} · {v.domain} · {v.required?'必需':'可选'} · {v.allow_dynamic?'支持未来成员':'固定成员'}<span className="mt-1 block text-muted-foreground">{v.purpose}</span></p>)}</div>}
              <div className="space-y-1 text-xs text-muted-foreground">{Object.entries({...p.manifest.required_capabilities,...p.manifest.optional_capabilities}).map(([name])=><p key={name}>{p.manifest.required_capabilities[name]?"必需":"可选"} · {name} · {list.capabilities.find(c=>c.name===name)?.availability==="available"?"服务可用":"服务未接入"}</p>)}{Object.keys(p.manifest.binding_schema).length>0&&<p>旧绑定契约：{Object.keys(p.manifest.binding_schema).join("、")}；其访问字段单独生效。</p>}</div>
              <details><summary className="cursor-pointer text-xs text-muted-foreground">文件与校验信息</summary><div className="mt-3 space-y-3">

              <ObjectHistory objectType="display_package" objectId={id} filter={{reference_id:p.id}} label="包管理历史"/><DomainFiles files={[{ref:{kind:'display.package',scope:{kind:'project',tenantId:scope.tenantId,projectId:scope.projectId},instanceId:id,packageId:p.id},display_name:`${p.manifest.name??p.manifest.app_key} ${p.manifest.version}`,media_type:'application/zip',sha256:p.sha256,state:p.state==='ready'?'available':p.state==='missing'?'missing':'preparing',allowed_actions:[]}]} actions={()=>null}/>
              <p className="text-sm">
                {p.manifest.files.length} 个文件 · 必需能力{" "}
                {Object.keys(p.manifest.required_capabilities).join("、") ||
                  "无"}
              </p></div></details>
              {project.can(P.ProjectDisplayUpload) && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!project.available || busy || !editable}
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
      {management && tab === "配置" && (
        <Configuration
          data={data}
          packageID={editingPackage}
          busy={!project.available || busy || !editable || !project.can(P.ProjectDisplayConfigure)}
          save={async (body) => { const result = await command("/configs", body); setTab("概览"); return result; }}
          runtime={list.runtime_origin}
        />
      )}
      {management && tab === "读取授权" && (
        <GrantForm
          key={data.grant?.revision ?? "new"}
          data={data}
          capabilities={list.capabilities}
          disabled={!project.available || busy || !project.can(P.ProjectDisplayGrant)}
          archived={!editable}
          save={(grant) => command("/grant", { grant }, "PUT")}
        />
      )}
      {management && (tab === '发布' || tab === '发布记录') && <Publication key={tab} data={data} review={tab === '发布'} editable={editable} busy={busy} command={command} onPublished={() => setTab('发布记录')} />}
      {management && tab === "运行与终端" && (
        <div className="space-y-3">
          <h3 className="text-sm font-semibold">运行会话</h3>
          {!data.sessions?.length && <EmptyState title="暂无运行会话" />}
          <p className="break-all text-xs text-muted-foreground">
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
      {management && tab === "运行与终端" && (
        <div className="space-y-4 border-t pt-5">
          <h3 className="text-sm font-semibold">固定终端</h3>
          {!data.terminals?.length && <p className="text-sm text-muted-foreground">暂无固定终端</p>}
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
              <Button disabled={!project.available || busy || !terminalName.trim()}>签发配对码（90 天）</Button>
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
                    disabled={!project.available || busy || !editable || t.state === "revoked"}
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
                    disabled={!project.available || busy || t.state === "revoked"}
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
                      disabled={!project.available || busy || !editable || t.state === "revoked"}
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
      {management && tab === "记录" && (
        <ol className="space-y-3">
          <li><ObjectHistory objectType="display_instance" objectId={id} /></li>
          {!data.events?.length && <li><EmptyState title="暂无运行记录" /></li>}
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
    </article>}
  </ResponsivePanel>;
}
