import type { MonitoringChannel, MonitoringRange } from '@infrapro/api-client';
export const qualityNames: Record<string, string> = { good: '正常', unmarked: '未标注', suspect: '可疑', bad: '无效' };
export const stateNames: Record<string, string> = { uploading: '上传中', received: '待映射／校验', validating: '全量校验中', needs_resolution: '需要核对', ready: '可发布', preparing: '准备生效', published: '已生效', completed_noop: '完成，无新增数据', failed: '失败', cancelled: '已取消', revoked: '已撤销', queued: '排队中', running: '执行中', succeeded: '已完成', needs_action: '等待处理', retry_wait: '等待重试', available: '可下载', invalidated: '已失效', expired: '结果过期' };
export const count = (value: string | undefined) => value === undefined ? '—' : BigInt(value).toLocaleString('zh-CN');
export const meta = (revision: string, reason: string) => ({ request_id: crypto.randomUUID(), expected_revision: revision, reason });
export const MIN_NS = -2208988800000000000n, MAX_NS = 9223372036854775807n;
const billion = 1000000000n;
export function parseNS(value: string): bigint {
  const match = value.match(/^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?(Z|[+-]\d\d:\d\d)$/);
  if (!match) throw new Error('时间须含秒和时区，例如 2020-01-01T00:00:00+08:00。');
  const millis = Date.parse(`${match[1]}${match[3]}`);
  if (!Number.isFinite(millis)) throw new Error('时间格式无效。');
  const result = BigInt(millis) * 1000000n + BigInt((match[2] ?? '').padEnd(9, '0'));
  if (result < MIN_NS || result > MAX_NS) throw new Error('时间超出 1900 至 2262 年的支持范围。');
  return result;
}
export function formatNS(value: bigint): string {
  const seconds = value >= 0n ? value / billion : (value - billion + 1n) / billion;
  return new Date(Number(seconds) * 1000).toISOString().replace('.000Z', `.${(value - seconds * billion).toString().padStart(9, '0')}Z`);
}
export function recentRange(latest: string, hours = 24): MonitoringRange {
  const end = parseNS(latest), from = end - BigInt(hours) * 3600n * billion;
  return { from: formatNS(from < MIN_NS ? MIN_NS : from), to: end === MAX_NS ? null : formatNS(end + 1n) };
}
export function rangeLabel(range: MonitoringRange) { return `${range.from ?? '不限开始'} 至 ${range.to ?? '不限结束'}（结束不含）`; }
export function decimalCoefficient(value: string): bigint {
  const negative = value.startsWith('-'), [integer, fraction = ''] = value.replace(/^[+-]/, '').split('.');
  if (!/^\d+$/.test(integer ?? '') || !/^\d{0,12}$/.test(fraction)) throw new Error('不支持的十进制原值。');
  return (negative ? -1n : 1n) * BigInt(`${integer}${fraction.padEnd(12, '0')}`);
}
export const compatibility = (channel: MonitoringChannel) => JSON.stringify([channel.source_kind, channel.metric_version_id || channel.output_contract_hash, channel.unit, channel.value_kind, channel.timestamp_meaning, channel.observation_direction, channel.reference_basis]);

export function shiftRange(range: MonitoringRange, shift: bigint): MonitoringRange {
  const from = range.from ? parseNS(range.from) : MIN_NS, to = range.to ? parseNS(range.to) : MAX_NS;
  let delta = shift;
  if (from + delta < MIN_NS) delta = MIN_NS - from;
  if (to + delta > MAX_NS) delta = MAX_NS - to;
  return { from: formatNS(from + delta), to: to + delta === MAX_NS ? null : formatNS(to + delta) };
}
export function zoomRange(range: MonitoringRange, numerator: bigint, denominator: bigint): MonitoringRange {
  const from = range.from ? parseNS(range.from) : MIN_NS, to = range.to ? parseNS(range.to) : MAX_NS;
  const half = (to - from) * numerator / denominator / 2n || 1n, middle = (from + to) / 2n;
  const left = middle - half < MIN_NS ? MIN_NS : middle - half, right = middle + half > MAX_NS ? MAX_NS : middle + half;
  return { from: formatNS(left), to: right === MAX_NS ? null : formatNS(right) };
}

export type TimeTile = { id: string; range: MonitoringRange; visible: boolean };
// A power-of-two grid anchored to the supported domain has at most three
// visible tiles. Neighbours share exact half-open edges even before 1970.
export function viewportTiles(range: MonitoringRange, width: number, curves: number) {
  const start = range.from ? parseNS(range.from) : MIN_NS;
  const end = range.to ? parseNS(range.to) : MAX_NS;
  const span = end > start ? end - start : 1n;
  let size = 1n; while (size * 2n < span) size *= 2n;
  const first = (start - MIN_NS) / size;
  const last = ((range.to ? end - 1n : end) - MIN_NS) / size;
  const plotWidth = Math.max(160, Math.min(4096, Math.floor(width / 128) * 128));
  const budget = Math.max(4, Math.floor(Math.min(2000, 20000 / Math.max(1, curves), plotWidth * 2) / 3));
  const tiles: TimeTile[] = [];
  for (let index = first > 0n ? first - 1n : first; index <= last + 1n; index++) {
    const lo = MIN_NS + index * size, hi = lo + size;
    if (lo > MAX_NS) break;
    tiles.push({ id: `${size}:${index}:${budget}`, range: { from: formatNS(lo), to: hi > MAX_NS ? null : formatNS(hi) }, visible: index >= first && index <= last });
  }
  return { tiles, budget, plotWidth };
}
