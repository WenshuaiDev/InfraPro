import type { components } from './schema';
type Semantics=components['schemas']['DeviceSemantics'];
// Matches Go unicode.IsSpace (including NEL, excluding BOM) and SQL's explicit
// character set. These texts are compared literally after newline normalization.
export function normalizeDeviceText(value:string):string{return value.replace(/\r\n/g,'\n').replace(/^\p{White_Space}+|\p{White_Space}+$/gu,'');}
export const deviceSemanticLabels={metric_version_id:'指标版本',unit_version_id:'单位版本',value_shape:'值形态',timestamp_meaning:'时间含义',aggregation_description:'统计含义',observation_direction:'观测方向',reference_basis:'参考基准'} as const;
export function deviceSemanticDifferences(output:Semantics,channel:Semantics):string[]{return (Object.keys(deviceSemanticLabels) as (keyof typeof deviceSemanticLabels)[]).filter(key=>{const a=output[key],b=channel[key];return ['aggregation_description','observation_direction','reference_basis'].includes(key)?normalizeDeviceText(String(a??''))!==normalizeDeviceText(String(b??'')):(a??null)!==(b??null);}).map(key=>`/${key}`);}
