import type { MonitoringQuery, MonitoringRange } from '@infrapro/api-client';
import { formatNS, MAX_NS, MIN_NS, parseNS } from './format';

export const allQualities: MonitoringQuery['qualities'] = ['good', 'unmarked', 'suspect', 'bad'];
export const qualityOptions = [{ value: 'default', label: '正常 + 未标注' }, { value: 'all', label: '全部质量' }, { value: 'suspect', label: '仅可疑' }, { value: 'bad', label: '仅无效' }];
export function selectedQualities(value: string): MonitoringQuery['qualities'] { return value === 'all' ? allQualities : value === 'suspect' ? ['suspect'] : value === 'bad' ? ['bad'] : ['good', 'unmarked']; }
export function latestRange(latest: string, minutes: number): MonitoringRange {
  const end = parseNS(latest), start = end - BigInt(minutes) * 60_000_000_000n;
  return { from: formatNS(start < MIN_NS ? MIN_NS : start), to: end === MAX_NS ? null : formatNS(end + 1n) };
}
export function displayTime(value: string | null | undefined, timezone: string, seconds = true) {
  if (!value) return '—';
  return new Intl.DateTimeFormat('zh-CN', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', ...(seconds ? { second: '2-digit' } : {}), hourCycle: 'h23' }).format(new Date(Number(parseNS(value) / 1_000_000n)));
}
export function observedAge(value: string, now: number) {
  const seconds = Math.floor((now - Number(parseNS(value) / 1_000_000n)) / 1000);
  if (seconds < -60) return '观测时间晚于当前时间';
  if (seconds < 60) return '1 分钟内观测';
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前观测`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前观测`;
  return `${Math.floor(seconds / 86400)} 天前观测`;
}
