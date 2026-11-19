import { reason, type Metric } from '@infrapro/diagnostics-client';
const labels: Record<string, string> = { healthy: '正常', degraded: '部分异常', unavailable: '不可用', unknown: '未知', disabled: '未启用', idle: '空闲', fresh: '新鲜', stale: '数据过期', never: '尚未采集', historical: '历史数据', clock_uncertain: '时间不可信', continuous: '常驻', on_demand: '按需', external: '外部服务', running: '运行中', stopped: '已退出', missing: '未找到进程', not_applicable: '不适用', ready: '就绪', not_ready: '未就绪', starting: '启动中', unconfigured: '未配置', working: '工作中', blocked: '循环失败', stalled: '循环停滞', complete: '完整', partial: '部分覆盖', none: '没有有效覆盖', physical: '物理主机', vm: '虚拟机', container: '容器', logical_test: '逻辑测试节点', host: '宿主资源', application: '应用', worker: '后台进程', database: '数据库', cache: '缓存', object_store: '对象存储', gateway: '网关', tool: '工具', diagnostics: '诊断', protocol: '协议检查', os: '操作系统', docker: 'Docker', systemd: 'systemd', self_report: '进程自身报告', durable: '已持久化', volatile: '未持久化', ok: '检查通过', failed: '检查失败', pending: '尚未执行', timeout: '检查超时', permission_denied: '无权限', unsupported: '不支持', not_configured: '未配置', invalid_report: '报告无效', resource: '基础资源', presence: '进程存活', readiness: '服务就绪', loop: '工作循环', self_report_mode: '源进程实际结果', shared_network: '同网络探测', external_probe: '外部网络视角' };
export function label(value: string) { return labels[value] ?? reason(value).title; }
export function timestamp(value?: string | null) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '尚未采集'; }
export function metricValue(metric: Metric) {
  if (metric.value === null || metric.availability !== 'available') return metric.availability === 'unlimited' ? '无限制' : metric.availability === 'permission_denied' ? '无权限' : metric.availability === 'unsupported' ? '不适用' : '未获取';
  if (metric.unit === 'position' && typeof metric.value === 'string') return metric.value;
  const value = Number(metric.value); if (!Number.isFinite(value)) return '未获取';
  if (metric.unit === 'bytes') { if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(1)} GiB`; if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(1)} MiB`; return `${value.toLocaleString()} B`; }
  if (metric.unit === 'ratio') return `${(value * 100).toFixed(1)}%`;
  if (metric.unit === 'cores') return `${value.toFixed(2)} 核`;
  if (metric.unit === 'bytes_per_second') return `${(value / 1024).toFixed(1)} KiB/s`;
  if (metric.unit === 'milliseconds') return `${value.toLocaleString()} 毫秒`;
  return value.toLocaleString();
}

export function resourceScope(value: string) { return value === 'none' ? '不统计资源' : label(value); }
