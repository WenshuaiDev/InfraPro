import type { DisplayBinding, DisplayConfig, DisplayDetail, DisplayGrant, DisplayInstance } from '@infrapro/api-client';

export type DisplayView = '概览' | '程序版本' | '发布记录' | '运行与终端' | '记录' | '配置' | '读取授权' | '设置' | '发布';
export type DisplayFilter = 'all' | 'published' | 'unpublished' | 'disabled' | 'archived';

export function matchesDisplay(instance: DisplayInstance, search: string, filter: DisplayFilter) {
  if (!instance.name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase())) return false;
  if (filter === 'disabled' || filter === 'archived') return instance.state === filter;
  if (filter === 'published') return instance.state === 'enabled' && !!instance.current_release_id;
  if (filter === 'unpublished') return instance.state === 'enabled' && !instance.current_release_id;
  return true;
}

// Missing management fields mean an intentionally restricted DTO, not empty data.
export function grantStatus(grant: DisplayGrant | undefined, now = Date.now()) {
  if (!grant) return '未授权';
  if (grant.revoked) return '已撤销';
  if (!Number.isFinite(Date.parse(grant.valid_until)) || Date.parse(grant.valid_until) <= now) return '已过期';
  return '有效';
}

export function displaySummary(data: DisplayDetail, enabled: boolean, now = Date.now()) {
  const { instance } = data;
  const management = data.packages !== undefined;
  const release = data.releases?.find(r => r.id === instance.current_release_id);
  const liveConfig = data.configs?.find(c => c.id === release?.config_id);
  const livePackage = data.packages?.find(p => p.id === release?.package_id);
  const draft = data.configs?.find(c => c.id === instance.draft_config_id);
  const draftPackage = data.packages?.find(p => p.id === draft?.package_id);
  const grant = management ? grantStatus(data.grant, now) : undefined;
  const status = !enabled ? '项目已停用' : instance.state === 'archived' ? '已归档' : instance.state === 'disabled' ? '已停用'
    : !instance.current_release_id ? '未发布' : release?.withdrawn ? '已撤回'
    : livePackage && livePackage.state !== 'ready' ? '程序不可用'
    : grant && grant !== '有效' ? (grant === '未授权' ? '授权未设置' : `授权${grant}`) : '已发布';
  return { management, release, liveConfig, livePackage, draft, draftPackage, grant, status, canOpen: status === '已发布' };
}

function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
  if (value && typeof value === 'object') return JSON.stringify(Object.keys(value).sort().map(k => [k, canonical((value as Record<string, unknown>)[k])]));
  return JSON.stringify(value) ?? '';
}

export function configChanges(live: DisplayConfig | undefined, draft: DisplayConfig | undefined) {
  if (!live || !draft) return [];
  return [
    live.package_id !== draft.package_id ? '程序版本' : '',
    canonical(live.parameters) !== canonical(draft.parameters) ? '展示参数' : '',
    canonical(live.bindings) !== canonical(draft.bindings) ? '数据绑定' : '',
    canonical(live.read_views) !== canonical(draft.read_views) ? '命名读取视图' : '',
    live.mode !== draft.mode ? '时间模式' : '',
  ].filter(Boolean);
}

export function bindingSummary(bindings: Record<string, DisplayBinding>) {
  const counts = { resources: new Set<string>(), channels: new Set<string>(), spatial: new Set<string>(), artifacts: new Set<string>() };
  let dynamic = false;
  for (const b of Object.values(bindings)) {
    b.ids.forEach(id => counts[b.kind].add(id));
    dynamic ||= !!b.dynamic;
  }
  const labels = { resources: '个资源', channels: '个通道', spatial: '个空间版本', artifacts: '个分析成果' };
  const parts = (Object.keys(counts) as (keyof typeof counts)[]).filter(k => counts[k].size).map(k => `${counts[k].size} ${labels[k]}`);
  if (dynamic) parts.push('动态通道范围');
  return parts.join(' · ') || '无数据绑定';
}

export function displayTime(value?: string) {
  return value && Number.isFinite(Date.parse(value)) ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—';
}
