import type { EngineeringContent } from '@infrapro/api-client';

export const facilityOrder = ['隧道', '桥梁', '路基', '路面', '涵洞', '边坡', '气象监测站'];
export const metricShapes: Record<string, string> = { integer: '整数', decimal: '小数', enum: '枚举' };
export const metricScope = { kind: 'metrics' } as const;

export function metricFacts(content: EngineeringContent) {
  const lines = content.description.trim().split(/\r?\n/).map(line => line.trim());
  const field = (prefix: string) => lines.find(line => new RegExp('^' + prefix + '[：:]').test(line))?.slice(prefix.length + 1).trim().split('。')[0] || '';
  const tags = content.metric?.facility_topics ?? field('适用设施').split(/[、，,；;]/u).map(tag => tag.trim()).filter(Boolean);
  return { topics: [...new Set(tags)], category: field('类别'), intro: field('定义') ? lines.find(line => /^定义[：:]/u.test(line))!.replace(/^定义[：:]\s*/u, '') : lines[0] || '' };
}
