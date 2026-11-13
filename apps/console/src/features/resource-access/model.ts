import type { ResourceGrant, ResourceKind } from '@infrapro/api-client';
export const kinds: Record<ResourceKind, string> = { facility: '设施', component: '构件', point: '测点', channel: '通道' };
export type Capability = ResourceGrant['capabilities'][number];
export const capabilities: Record<ResourceKind, Capability[]> = {facility:['read','update','create_component','create_point','create_derived_channel'],component:['read','update','create_component','create_point','lifecycle','create_derived_channel'],point:['read','update','create_channel','lifecycle','create_derived_channel'],channel:['read','update','lifecycle','source_manage','manage_calculation']};
export const capNames: Record<string,string> = {create_derived_channel:'新增直属派生通道',manage_calculation:'维护派生计算（仅派生通道）',source_manage:'维护来源',read:'读取',update:'编辑',create_component:'新增构件',create_point:'新增测点',create_channel:'新增通道',lifecycle:'退役/恢复'};
