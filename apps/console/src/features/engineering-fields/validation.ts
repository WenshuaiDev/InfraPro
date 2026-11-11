import type { EngineeringField, EngineeringIssue } from '@infrapro/api-client';
import { z } from '../../config/validation';
const maxNumber = Number.MAX_SAFE_INTEGER;
const issue = (code: string, message: string, path: string, key = ''): EngineeringIssue => ({ code, message, path, key, severity: 'error' });
function dateValid(value: string) { if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false; const [y, m, d] = value.split('-').map(Number); if (m! < 1 || m! > 12 || d! < 1) return false; const leap = y! % 4 === 0 && (y! % 100 !== 0 || y! % 400 === 0); return d! <= [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][m! - 1]!; }
function dateTimeValid(value: string) { const match = /^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-](\d{2}):(\d{2}))$/.exec(value); return !!match && dateValid(match[1]!) && Number(match[2]) < 24 && Number(match[3]) < 60 && Number(match[4]) < 60 && (match[5] === 'Z' || (Number(match[6]) < 24 && Number(match[7]) < 60)); }
export function fieldRule(f: EngineeringField): z.ZodType {
 let rule: z.ZodType;
 switch (f.shape) {
 case 'text': rule = z.string().refine((v) => Array.from(v).length >= Math.max(f.min_length ?? 0, f.required ? 1 : 0) && Array.from(v).length <= (f.max_length ?? 10000)); break;
 case 'integer': case 'decimal': rule = z.number().refine((v) => Number.isFinite(v) && Math.abs(v) <= maxNumber && (f.shape !== 'integer' || Number.isSafeInteger(v)) && (f.minimum === undefined || v >= f.minimum) && (f.maximum === undefined || v <= f.maximum)); break;
 case 'boolean': rule = z.boolean(); break;
 case 'date': rule = z.string().refine(dateValid); break;
 case 'datetime': rule = z.string().refine(dateTimeValid); break;
 case 'enum': rule = z.string().refine((v) => (f.options ?? []).some((o) => o.key === v)); break;
 case 'multi_enum': rule = z.array(z.string()).refine((v) => new Set(v).size === v.length && v.every((k) => (f.options ?? []).some((o) => o.key === k)) && v.length >= Math.max(f.min_items ?? 0, f.required ? 1 : 0) && v.length <= (f.max_items ?? f.options?.length ?? 0)); break;
 }
 return f.required ? rule : rule.optional();
}
export function validateFields(fields: EngineeringField[], values: Record<string, unknown>, path: string): EngineeringIssue[] {
 const issues: EngineeringIssue[] = [];
 for (const f of fields) { if (!fieldRule(f).safeParse(values[f.key]).success) issues.push(issue(values[f.key] === undefined ? 'required' : 'invalid_value', values[f.key] === undefined ? '请填写必填属性。' : '值不符合属性的数据形态、选项或范围约束。', `${path}/${f.key}`, f.key)); }
 for (const k of Object.keys(values)) if (!fields.some((f) => f.key === k)) issues.push(issue('unknown_property', '该属性未在定义中声明。', `${path}/${k}`, k));
 return issues;
}
export const defaults = (fields: EngineeringField[]) => Object.fromEntries(fields.filter((f) => f.default !== undefined).map((f) => [f.key, structuredClone(f.default)]));
