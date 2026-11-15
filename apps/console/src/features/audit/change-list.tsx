import type { AuditChange } from '@infrapro/api-client';
const labels:Record<string,string>={name:'名称',display_name:'显示名称',description:'说明',status:'状态',enabled:'是否启用',permissions:'权限',permission_codes:'权限编码',roles:'角色',role_ids:'角色',reason:'操作原因',version:'修订',current_revision:'当前文件修订',before:'变更前',after:'变更后',count:'数量',state:'执行状态',filename:'文件名称',size_bytes:'字节数'};
function Value({value,state}:{value:unknown;state:AuditChange['before_state']}) {
 if(state==='not_recorded')return <span className="text-muted-foreground">未记录</span>;
 if(state==='redacted')return <span className="text-muted-foreground">已脱敏</span>;
 const text=value===null?'空值（null）':Array.isArray(value)&&value.length===0?'空集合':typeof value==='string'?(value||'空文本'):typeof value==='boolean'?(value?'是':'否'):JSON.stringify(value);
 return <span className="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{text}{state==='truncated'&&<small className="block text-muted-foreground">内容已按上限截断</small>}</span>;
}
export function ChangeList({ changes }: { changes: AuditChange[] }) {
  return <section className="space-y-3"><h3 className="text-sm font-semibold">实际变更{changes.length > 0 && <span className="ml-2 rounded bg-muted px-1.5 text-xs font-normal text-muted-foreground">{changes.length} 项</span>}</h3>{!changes.length ? <p className="rounded-lg bg-muted/30 p-4 text-sm leading-6 text-muted-foreground">没有可展示的字段差异；可能是历史未记录或当前权限已裁剪。</p> : <div className="overflow-hidden rounded-lg border"><div className="grid grid-cols-[1fr_1.3fr_1.3fr] bg-muted/40 text-xs text-muted-foreground"><p className="p-3">字段</p><p className="p-3">变更前</p><p className="p-3">变更后</p></div>{changes.map(c => <div key={c.key} className="grid grid-cols-[1fr_1.3fr_1.3fr] border-t text-xs leading-6"><div className="min-w-0 break-words p-3 font-medium">{labels[c.key] ?? c.label}</div><div className="min-w-0 p-3 text-muted-foreground"><Value value={c.before} state={c.before_state} /></div><div className="min-w-0 bg-emerald-50/60 p-3 dark:bg-emerald-950/20"><Value value={c.after} state={c.after_state} /></div></div>)}</div>}</section>;
}
