import type { EngineeringIssue } from '@infrapro/api-client';
import { z } from '../../config/validation';
const text = (max=200) => z.string().refine((v) => Array.from(v).length <= max);
const required = (max=200) => text(max).refine((v) => v.trim().length>0);
const number = z.number().finite().min(-Number.MAX_SAFE_INTEGER).max(Number.MAX_SAFE_INTEGER);
const unit = z.enum(['m','mm']);
const geo = z.strictObject({crs:required(),longitude:number.min(-180).max(180),latitude:number.min(-90).max(90)});
const projected = z.strictObject({crs:required(),x:number,y:number,axis_description:required(2000),unit});
const elevation = z.strictObject({value_m:number,datum:required()});
const local = z.strictObject({reference_description:required(2000),axes_description:required(2000),unit,x:number,y:number,z:number.optional()});
const linear = z.strictObject({route_code:required(),route_name:text().optional(),reference_name:text().optional(),start_station:text().optional(),end_station:text().optional(),side:text().optional(),direction:text().optional(),offset_m:number.optional(),note:text(2000).optional()}).refine((v) => !v.end_station || !!v.start_station?.trim(),{path:['start_station'],message:'填写终点桩号时须提供起点桩号。'});
export const positionRule = z.strictObject({description:text(2000).optional(),linear_references:z.array(linear).max(20).optional(),geographic:geo.nullish(),projected:projected.nullish(),elevation:elevation.nullish(),local:local.nullish(),sections:z.array(z.strictObject({kind:z.enum(['route_cross_section','facility_monitoring_section']),code:required(),name:text().optional(),reference_description:required(2000)})).max(20).optional(),source_reference:text(1000).optional(),source_date:z.iso.date().nullish(),accuracy_m:number.min(0).optional()});
export function positionIssues(value: unknown): EngineeringIssue[] { const parsed = positionRule.safeParse(value); const issues: EngineeringIssue[] = parsed.success ? [] : parsed.error.issues.map((v) => ({code:'position_invalid',path:`/position/${v.path.join('/')}`,message:v.message.startsWith('填写') ? v.message : '请填写有效数值、明确参照或符合长度限制的资料。',severity:'error',key:''})); if (new TextEncoder().encode(JSON.stringify(value)).length>32*1024) issues.push({code:'input_limit',path:'/position',message:'位置内容最多 32 KiB。',severity:'error',key:''}); return issues; }
