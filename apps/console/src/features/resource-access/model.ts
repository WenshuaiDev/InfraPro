import type {
  ResourceAccessDetail,
  ResourceGrant,
  ResourceGrantInput,
  ResourceKind,
} from '@infrapro/api-client';
export const kinds: Record<ResourceKind, string> = {
  facility: '设施',
  component: '构件', monitoring_section: '监测断面', borehole: '孔位',
  point: '测点',
  channel: '通道',
};
export type Capability = ResourceGrant['capabilities'][number];
export const capabilities: Record<ResourceKind, Capability[]> = {
  facility: ['read', 'update', 'create_component', 'create_monitoring_section', 'create_borehole', 'create_point', 'create_derived_channel'],
  component: ['read', 'update', 'create_component', 'create_monitoring_section', 'create_borehole', 'create_point', 'create_derived_channel', 'lifecycle'],
  monitoring_section: ['read','update','create_component','create_monitoring_section','create_borehole','create_point','create_derived_channel','lifecycle'],
  borehole: ['read','update','create_component','create_monitoring_section','create_borehole','create_point','create_derived_channel','lifecycle'],
  point: ['read', 'update', 'create_channel', 'create_derived_channel', 'lifecycle'],
  channel: ['read', 'update', 'lifecycle', 'source_manage', 'manage_calculation'],
};
export const capNames: Record<Capability, string> = {
  source_manage: '维护通道来源',
  create_derived_channel: '新增直属派生通道',
  manage_calculation: '维护派生计算（仅派生通道）',
  read: '查看',
  update: '编辑资料',
  create_component: '新增构件',
  create_point: '新增测点',
  create_monitoring_section: '新增监测断面',
  create_borehole: '新增孔位',
  create_channel: '新增通道',
  lifecycle: '退役与恢复',
};
export type Choice = {
  kind: ResourceKind;
  id: string;
  name: string;
  code: string;
  status?: string;
  source_kind?: 'direct' | 'derived';
  capabilities: Capability[];
  grant?: ResourceGrant;
  end?: boolean;
};
export type Draft = Record<string, Choice>;
export type Summary = ResourceAccessDetail['summary'];
export const emptySummary: Summary = {
  facilities: 0,
  components: 0, monitoring_sections: 0, boreholes: 0,
  points: 0,
  channels: 0,
  maintainable: 0,
};
export const countResources = (s: Summary) => s.facilities + s.components + s.monitoring_sections + s.boreholes + s.points + s.channels;
export const describe = (caps?: readonly Capability[]) =>
  caps?.length ? caps.map((c) => capNames[c]).join('、') : '未授权';
export const sameCaps = (a: readonly Capability[], b: readonly Capability[]) =>
  a.length === b.length && a.every((c) => b.includes(c));
export const asChoice = (g: ResourceGrant): Choice => ({
  kind: g.kind,
  id: g.resource_id,
  name: g.name,
  code: g.code,
  source_kind: g.source_kind,
  capabilities: g.capabilities,
  grant: g,
});
export function grantInput(v: Choice): ResourceGrantInput {
  return {
    kind: v.kind,
    capabilities: v.capabilities,
    ...(v.kind === 'monitoring_section' ? {monitoring_section_id:v.id} : v.kind === 'borehole' ? {borehole_id:v.id} : v.kind === 'component'
      ? { component_id: v.id }
      : v.kind === 'point'
        ? { point_id: v.id }
        : v.kind === 'channel'
          ? { channel_id: v.id }
          : {}),
  };
}
// Store only changes. Restoring the original capability set clears the pending change.
export function putChoice(draft: Draft, value: Choice): Draft {
  const next = { ...draft };
  if (
    (value.end && !value.grant) ||
    (!value.end && value.grant && sameCaps(value.capabilities, value.grant.capabilities))
  )
    delete next[value.id];
  else next[value.id] = value;
  return next;
}
export function draftSummary(original: Summary, choices: Choice[]): Summary {
  const next = { ...original };
  const key = {
    facility: 'facilities',
    component: 'components', monitoring_section: 'monitoring_sections', borehole: 'boreholes',
    point: 'points',
    channel: 'channels',
  } as const;
  for (const v of choices) {
    if (!v.grant && !v.end) next[key[v.kind]]++;
    if (v.grant && v.end) next[key[v.kind]]--;
    next.maintainable +=
      Number(!v.end && v.capabilities.length > 1) - Number(!!v.grant && v.grant.capabilities.length > 1);
  }
  return next;
}
export function assertAccessRevision(data: ResourceAccessDetail, revision: string) {
  if (data.policy.revision !== revision)
    throw new Error('授权已被其他操作更新。当前选择已保留，请重新载入后复核。');
  return data;
}
