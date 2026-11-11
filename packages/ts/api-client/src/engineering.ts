import type { components } from './schema';
type Schema<K extends keyof components['schemas']> = components['schemas'][K];
export type EngineeringContent = Schema<'EngineeringContent'>;
export type EngineeringDefinition = Schema<'EngineeringDefinition'>;
export type EngineeringVersion = Schema<'EngineeringVersion'>;
export type EngineeringField = Schema<'EngineeringField'>;
export type EngineeringComponent = Schema<'EngineeringComponent'>;
export type EngineeringRelation = Schema<'EngineeringRelation'>;
export type EngineeringMetric = Schema<'EngineeringMetric'>;
export type EngineeringTemplate = Schema<'EngineeringTemplate'>;
export type EngineeringItem = Schema<'EngineeringItem'>;
export type EngineeringBinding = Schema<'EngineeringBinding'>;
export type EngineeringOption = Schema<'EngineeringOption'>;
export type EngineeringIssue = Schema<'EngineeringIssue'>;
export type EngineeringPreview = Schema<'EngineeringPreview'>;
export type EngineeringSample = Schema<'EngineeringSample'>;
export type EngineeringCategory = Schema<'EngineeringCategory'>;
export type EngineeringResult = Schema<'EngineeringResult'>;
export type EngineeringSummary = Schema<'EngineeringSummary'>;
export type EngineeringDifference = Schema<'EngineeringDifference'>;
export type EngineeringKind = EngineeringDefinition['kind'];
export type EngineeringScope = { kind: EngineeringKind; tenant?: string; catalog?: boolean };
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal) => Promise<T>;
const base = (s: EngineeringScope) => `${s.tenant ? `/tenants/${encodeURIComponent(s.tenant)}` : '/platform'}/engineering/${s.catalog ? 'catalog/' : ''}${s.kind}`;
const categoryPath = (tenant?: string) => `${tenant ? `/tenants/${encodeURIComponent(tenant)}` : '/platform'}/engineering/unit-categories`;
const query = (filter: Record<string, string | number | undefined>) => new URLSearchParams(Object.entries(filter).filter(([,v]) => v !== undefined && v !== '').map(([k,v]) => [k,String(v)])).toString();
export function engineeringMethods(request: Request) {
  return {
    engineeringReceipt: (scope: EngineeringScope, id: string, signal?: AbortSignal) => request<EngineeringResult>(`${base(scope)}/requests/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
    engineeringList: (scope: EngineeringScope, filter: { q?: string; status?: string; offset?: number; limit?: number } = {}, signal?: AbortSignal) => request<Schema<'EngineeringPage'>>(`${base(scope)}?${query(filter)}`, 'GET', undefined, true, signal),
    engineeringDetail: (scope: EngineeringScope, id: string, signal?: AbortSignal) => request<EngineeringDefinition>(`${base(scope)}/${encodeURIComponent(id)}`, 'GET', undefined, true, signal),
    engineeringCreate: (scope: EngineeringScope, input: Schema<'EngineeringCreateInput'>) => request<EngineeringResult>(base(scope), 'POST', input),
    engineeringSave: (scope: EngineeringScope, id: string, input: Schema<'EngineeringSaveInput'>) => request<EngineeringResult>(`${base(scope)}/${id}/draft`, 'PUT', input),
    engineeringNewDraft: (scope: EngineeringScope, id: string, input: Schema<'EngineeringNewDraftInput'>) => request<EngineeringResult>(`${base(scope)}/${id}/draft`, 'POST', input),
    engineeringDeleteDraft: (scope: EngineeringScope, id: string, input: Schema<'EngineeringRevisionInput'>) => request<EngineeringResult>(`${base(scope)}/${id}/draft/delete`, 'POST', input),
    engineeringCopy: (scope: EngineeringScope, input: Schema<'EngineeringCopyInput'>) => request<EngineeringResult>(`${base(scope)}/copy`, 'POST', input),
    engineeringPublish: (scope: EngineeringScope, id: string, input: Schema<'EngineeringRevisionInput'>) => request<EngineeringResult>(`${base(scope)}/${id}/publish`, 'POST', input),
    engineeringState: (scope: EngineeringScope, id: string, version: string, input: Schema<'EngineeringStateInput'>) => request<EngineeringResult>(`${base(scope)}/${id}/versions/${version}/state`, 'POST', input),
    engineeringPreview: (scope: EngineeringScope, id: string, input: Schema<'EngineeringPreviewInput'>, signal?: AbortSignal) => request<EngineeringPreview>(`${base(scope)}/${id}/preview`, 'POST', input, true, signal),
    engineeringDiff: (scope: EngineeringScope, id: string, from: string, to: string, revision: number, signal?: AbortSignal) => request<EngineeringDifference[]>(`${base(scope)}/${id}/diff?${query({ from, to, revision })}`, 'GET', undefined, true, signal),
    engineeringImpact: (scope: EngineeringScope, id: string, version: string, signal?: AbortSignal) => request<Schema<'EngineeringImpact'>>(`${base(scope)}/${id}/versions/${version}/impact`, 'GET', undefined, true, signal),
    engineeringCategories: (tenant?: string, signal?: AbortSignal) => request<EngineeringCategory[]>(categoryPath(tenant), 'GET', undefined, true, signal),
    engineeringSaveCategory: (input: Schema<'EngineeringCategoryInput'>) => request<EngineeringResult>(categoryPath(), 'POST', input),
    engineeringDeleteCategory: (key: string, input: Schema<'EngineeringRevisionInput'>) => request<EngineeringResult>(`${categoryPath()}/${encodeURIComponent(key)}/delete`, 'POST', input),
  };
}
