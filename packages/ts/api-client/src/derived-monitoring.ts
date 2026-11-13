import type { components } from './schema';
import { monitoringRoot, type MonitoringRequestOptions, type MonitoringScope } from './monitoring';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];
export type DerivedScope = MonitoringScope;
export type DerivedGeneration = Schema<'DerivedGeneration'>;
export type DerivedGenerationQuery = Schema<'DerivedGenerationQuery'>;
export type DerivedGenerationFilter = { from_ns: string; to_ns?: string; cursor?: string; limit?: number };
export type DerivedObservation = Schema<'DerivedObservation'>;
export type DerivedCoverage = Schema<'DerivedCoverage'>;
export type DerivedRule = Schema<'DerivedRule'>;
export type DerivedQuantity = Schema<'DerivedQuantity'>;
export type DerivedInputSpec = Schema<'DerivedInputSpec'>;
export type DerivedMetricDefinition = Schema<'DerivedMetricDefinition'>;
export type DerivedMetricVersion = Schema<'DerivedMetricVersion'>;
export type DerivedMetricPage = Schema<'DerivedMetricPage'>;
export type DerivedVersionPage = Schema<'DerivedVersionPage'>;
export type DerivedMetricResult = Schema<'DerivedMetricResult'>;
export type DerivedTemplatePage = Schema<'DerivedTemplatePage'>;
export type DerivedRuleValidation = Schema<'DerivedRuleValidation'>;
export type DerivedIssue = Schema<'DerivedIssue'>;
export type DerivedCreateMetricInput = Schema<'DerivedCreateMetricInput'>;
export type DerivedSaveMetricInput = Schema<'DerivedSaveMetricInput'>;
export type DerivedPublishMetricInput = Schema<'DerivedPublishMetricInput'>;
export type DerivedChannel = Schema<'DerivedChannel'>;
export type DerivedCreateChannelInput = Schema<'DerivedCreateChannelInput'>;
export type DerivedChannelConfig = Schema<'DerivedChannelConfig'>;
export type DerivedRun = Schema<'DerivedRun'>;
export type DerivedTrialInput = Schema<'DerivedTrialInput'>;
export type DerivedTrialResult = Schema<'DerivedTrialResult'>;
export type AnalysisExportInput = Schema<'AnalysisExportInput'>;
export type GenerationExportInput = Schema<'GenerationExportInput'>;
export type GenerationExportRecord = Schema<'GenerationExportRecord'>;
export type GenerationExportManifest = Schema<'GenerationExportManifest'>;
export type AnalysisExportRecord = Schema<'AnalysisExportRecord'>;
export type AnalysisExportManifest = Schema<'AnalysisExportManifest'>;
export type AnalysisBody = Schema<'AnalysisBody'>;
export type AnalysisConfig = Schema<'AnalysisConfig'>;
export type AnalysisVersion = Schema<'AnalysisVersion'>;
export type AnalysisConfigResult = Schema<'AnalysisConfigResult'>;
export type AnalysisValidation = Schema<'AnalysisValidation'>;
export type AnalysisRunInput = Schema<'AnalysisRunInput'>;
export type AnalysisArtifact = Schema<'AnalysisArtifact'>;
export type AnalysisArtifactVersion = Schema<'AnalysisArtifactVersion'>;
export type AnalysisArtifactDetail = Schema<'AnalysisArtifactDetail'>;
export type AnalysisArtifactQuery = Schema<'AnalysisArtifactQuery'>;
export type AnalysisArtifactRows = Schema<'AnalysisArtifactRows'>;
export type AnalysisCompareInput = Schema<'AnalysisCompareInput'>;
export type AnalysisComparisonResult = Schema<'AnalysisComparisonResult'>;
export type AnalysisComparisonRows = Schema<'AnalysisComparisonRows'>;
export type AnalysisFilter = { q?: string; state?: 'active' | 'archived'; cursor?: string; limit?: number };
export type DerivedControlPreview = Schema<'DerivedControlPreview'>;
export type DerivedControlPreviewInput = Schema<'DerivedControlPreviewInput'>;
export type DerivedControlInput = Schema<'DerivedControlInput'>;
export type DerivedSaveConfigInput = Schema<'DerivedSaveConfigInput'>;
export type DerivedConfigPreviewInput = Schema<'DerivedConfigPreviewInput'>;
export type DerivedConfigPreview = Schema<'DerivedConfigPreview'>;
export type DerivedRunFilter = { channel_id?: string; state?: Schema<'MonitoringJob'>['state']; cursor?: string; limit?: number };
export type DerivedChannelFilter = { q?: string; cursor?: string; limit?: number; facility_id?: string; owner_kind?: 'facility' | 'component' | 'point'; owner_id?: string; control_state?: 'draft' | 'enabled' | 'paused' | 'retired' };
export type DerivedFilter = { q?: string; state?: 'published' | 'draft'; cursor?: string; limit?: number };
type TenantScope = Extract<DerivedScope, { kind: 'tenant' }>;
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal, options?: MonitoringRequestOptions) => Promise<T>;
const id = encodeURIComponent;
export const derivedRoot = (scope: DerivedScope) => `${monitoringRoot(scope)}/derived`;
const query = (values: DerivedFilter | DerivedChannelFilter | DerivedRunFilter | DerivedGenerationFilter | AnalysisFilter | {version_id?: string}) => {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) if (value !== undefined && value !== '') params.set(key, String(value));
  const result = params.toString();
  return result ? `?${result}` : '';
};

export function derivedMonitoringMethods(request: Request, base: string) {
  const call = <T>(scope: DerivedScope, path: string, method = 'GET', body?: unknown, signal?: AbortSignal) => request<T>(`${derivedRoot(scope)}${path}`, method, body, true, signal, { timeoutMs: 30000 });
  return {
    generationCreateExport: (scope: DerivedScope, artifact: string, input: GenerationExportInput, signal?: AbortSignal) => call<GenerationExportRecord>(scope, `/generations/${id(artifact)}/exports`, 'POST', input, signal),
    generationExports: (scope: DerivedScope, filter: {cursor?:string;limit?:number} = {}, signal?: AbortSignal) => call<Schema<'GenerationExportPage'>>(scope, `/evidence/exports${query(filter)}`, 'GET', undefined, signal),
    generationExport: (scope: DerivedScope, exported: string, signal?: AbortSignal) => call<GenerationExportRecord>(scope, `/evidence/exports/${id(exported)}`, 'GET', undefined, signal),
    generationExportFiles: (scope: DerivedScope, exported: string, signal?: AbortSignal) => call<GenerationExportManifest>(scope, `/evidence/exports/${id(exported)}/files`, 'GET', undefined, signal),
    generationExportDownloadPath: (scope: DerivedScope, exported: string, part: string) => `${base}${derivedRoot(scope)}/evidence/exports/${id(exported)}/files/${id(part)}`,
    generationCancelExport: (scope: DerivedScope, exported: string, input: Schema<'AnalysisWriteMeta'>, signal?: AbortSignal) => call<GenerationExportRecord>(scope, `/evidence/exports/${id(exported)}/cancel`, 'POST', input, signal),
    generationRetryExport: (scope: DerivedScope, exported: string, input: Schema<'AnalysisWriteMeta'>, signal?: AbortSignal) => call<GenerationExportRecord>(scope, `/evidence/exports/${id(exported)}/retry`, 'POST', input, signal),
    analysisCreateExport: (scope: DerivedScope, artifact: string, input: AnalysisExportInput, signal?: AbortSignal) => call<AnalysisExportRecord>(scope, `/analysis/artifacts/${id(artifact)}/exports`, 'POST', input, signal),
    analysisExports: (scope: DerivedScope, filter: {cursor?:string;limit?:number} = {}, signal?: AbortSignal) => call<Schema<'AnalysisExportPage'>>(scope, `/analysis/exports${query(filter)}`, 'GET', undefined, signal),
    analysisExport: (scope: DerivedScope, exported: string, signal?: AbortSignal) => call<AnalysisExportRecord>(scope, `/analysis/exports/${id(exported)}`, 'GET', undefined, signal),
    analysisExportFiles: (scope: DerivedScope, exported: string, signal?: AbortSignal) => call<AnalysisExportManifest>(scope, `/analysis/exports/${id(exported)}/files`, 'GET', undefined, signal),
    analysisExportDownloadPath: (scope: DerivedScope, exported: string, part: string) => `${base}${derivedRoot(scope)}/analysis/exports/${id(exported)}/files/${id(part)}`,
    analysisCancelExport: (scope: DerivedScope, exported: string, input: Schema<'AnalysisWriteMeta'>, signal?: AbortSignal) => call<AnalysisExportRecord>(scope, `/analysis/exports/${id(exported)}/cancel`, 'POST', input, signal),
    analysisRetryExport: (scope: DerivedScope, exported: string, input: Schema<'AnalysisWriteMeta'>, signal?: AbortSignal) => call<AnalysisExportRecord>(scope, `/analysis/exports/${id(exported)}/retry`, 'POST', input, signal),
    analysisCompare: (scope: DerivedScope, input: AnalysisCompareInput, signal?: AbortSignal) => call<DerivedRun>(scope, '/analysis/comparisons', 'POST', input, signal),
    analysisComparisonRows: (scope: DerivedScope, run: string, input: Schema<'AnalysisComparisonQuery'> = {}, signal?: AbortSignal) => call<AnalysisComparisonRows>(scope, `/analysis/comparisons/${id(run)}/query`, 'POST', input, signal),
    analysisConfigs: (scope: DerivedScope, filter: AnalysisFilter = {}, signal?: AbortSignal) => call<Schema<'AnalysisConfigPage'>>(scope, `/analysis/configs${query(filter)}`, 'GET', undefined, signal),
    analysisConfig: (scope: DerivedScope, config: string, signal?: AbortSignal) => call<AnalysisConfig>(scope, `/analysis/configs/${id(config)}`, 'GET', undefined, signal),
    analysisCreateConfig: (scope: DerivedScope, input: Schema<'AnalysisCreateInput'>, signal?: AbortSignal) => call<AnalysisConfigResult>(scope, '/analysis/configs', 'POST', input, signal),
    analysisSaveConfig: (scope: DerivedScope, config: string, input: Schema<'AnalysisSaveInput'>, signal?: AbortSignal) => call<AnalysisConfigResult>(scope, `/analysis/configs/${id(config)}/draft`, 'PATCH', input, signal),
    analysisCopyConfig: (scope: DerivedScope, config: string, input: Schema<'AnalysisCopyInput'>, signal?: AbortSignal) => call<AnalysisConfigResult>(scope, `/analysis/configs/${id(config)}/copy`, 'POST', input, signal),
    analysisValidateConfig: (scope: DerivedScope, config: string, input: Schema<'DerivedValidateInput'>, signal?: AbortSignal) => call<AnalysisValidation>(scope, `/analysis/configs/${id(config)}/validate`, 'POST', input, signal),
    analysisPublishConfig: (scope: DerivedScope, config: string, input: Schema<'AnalysisPublishInput'>, signal?: AbortSignal) => call<AnalysisConfigResult>(scope, `/analysis/configs/${id(config)}/versions`, 'POST', input, signal),
    analysisConfigVersions: (scope: DerivedScope, config: string, filter: {cursor?: string} = {}, signal?: AbortSignal) => call<Schema<'AnalysisVersionPage'>>(scope, `/analysis/configs/${id(config)}/versions${query(filter)}`, 'GET', undefined, signal),
    analysisArchiveConfig: (scope: DerivedScope, config: string, input: Schema<'AnalysisWriteMeta'>, signal?: AbortSignal) => call<AnalysisConfigResult>(scope, `/analysis/configs/${id(config)}/archive`, 'POST', input, signal),
    analysisTrial: (scope: DerivedScope, input: AnalysisRunInput, signal?: AbortSignal) => call<DerivedRun>(scope, '/analysis/trials', 'POST', input, signal),
    analysisStartRun: (scope: DerivedScope, input: AnalysisRunInput, signal?: AbortSignal) => call<DerivedRun>(scope, '/analysis/runs', 'POST', input, signal),
    analysisArtifacts: (scope: DerivedScope, filter: AnalysisFilter = {}, signal?: AbortSignal) => call<Schema<'AnalysisArtifactPage'>>(scope, `/analysis/artifacts${query(filter)}`, 'GET', undefined, signal),
    analysisArtifact: (scope: DerivedScope, artifact: string, version?: string, signal?: AbortSignal) => call<AnalysisArtifactDetail>(scope, `/analysis/artifacts/${id(artifact)}${query({version_id: version})}`, 'GET', undefined, signal),
    analysisArtifactVersions: (scope: DerivedScope, artifact: string, filter: {cursor?: string} = {}, signal?: AbortSignal) => call<Schema<'AnalysisArtifactVersions'>>(scope, `/analysis/artifacts/${id(artifact)}/versions${query(filter)}`, 'GET', undefined, signal),
    analysisArtifactRows: (scope: DerivedScope, artifact: string, input: AnalysisArtifactQuery, signal?: AbortSignal) => call<AnalysisArtifactRows>(scope, `/analysis/artifacts/${id(artifact)}/query`, 'POST', input, signal),
    analysisArtifactLineage: (scope: DerivedScope, artifact: string, input: AnalysisArtifactQuery, signal?: AbortSignal) => call<Schema<'AnalysisLineage'>>(scope, `/analysis/artifacts/${id(artifact)}/lineage`, 'POST', input, signal),
    analysisArchiveArtifact: (scope: DerivedScope, artifact: string, input: Schema<'AnalysisWriteMeta'>, signal?: AbortSignal) => call<Schema<'AnalysisArtifactArchiveResult'>>(scope, `/analysis/artifacts/${id(artifact)}/archive`, 'POST', input, signal),
    derivedReplayGeneration: (scope: DerivedScope, generation: string, input: Schema<'DerivedReplayInput'>, signal?: AbortSignal) => call<DerivedRun>(scope, `/generations/${id(generation)}/replay`, 'POST', input, signal),
    derivedGenerations: (scope: DerivedScope, channel: string, filter: DerivedGenerationFilter, signal?: AbortSignal) => call<Schema<'DerivedGenerationPage'>>(scope, `/channels/${id(channel)}/generations${query(filter)}`, 'GET', undefined, signal),
    derivedGenerationDetails: (scope: DerivedScope, generation: string, input: DerivedGenerationQuery = {}, signal?: AbortSignal) => call<Schema<'DerivedGenerationDetails'>>(scope, `/generations/${id(generation)}/query`, 'POST', input, signal),
    derivedGenerationLineage: (scope: DerivedScope, generation: string, input: DerivedGenerationQuery = {}, signal?: AbortSignal) => call<Schema<'DerivedGenerationLineage'>>(scope, `/generations/${id(generation)}/lineage`, 'POST', input, signal),
    derivedBackfillPreview: (scope: DerivedScope, channel: string, input: Schema<'DerivedBackfillPreviewInput'>, signal?: AbortSignal) => call<Schema<'DerivedBackfillPreview'>>(scope, `/channels/${id(channel)}/backfill-preview`, 'POST', input, signal),
    derivedBackfill: (scope: DerivedScope, channel: string, input: DerivedControlInput, signal?: AbortSignal) => call<Schema<'DerivedBackfillResult'>>(scope, `/channels/${id(channel)}/backfill`, 'POST', input, signal),
    derivedSaveConfig: (scope: DerivedScope, channel: string, input: DerivedSaveConfigInput, signal?: AbortSignal) => call<Schema<'DerivedConfigResult'>>(scope, `/channels/${id(channel)}/configs`, 'POST', input, signal),
    derivedConfigPreview: (scope: DerivedScope, channel: string, input: DerivedConfigPreviewInput, signal?: AbortSignal) => call<DerivedConfigPreview>(scope, `/channels/${id(channel)}/config-preview`, 'POST', input, signal),
    derivedApplyConfig: (scope: DerivedScope, channel: string, input: DerivedControlInput, signal?: AbortSignal) => call<Schema<'DerivedChannelResult'>>(scope, `/channels/${id(channel)}/apply-config`, 'POST', input, signal),
    derivedAssignments: (scope: DerivedScope, channel: string, revision?: string, cursor?: string, signal?: AbortSignal) => call<Schema<'DerivedAssignmentPage'>>(scope, `/channels/${id(channel)}/assignments?${new URLSearchParams({ ...(revision ? { revision } : {}), ...(cursor ? { cursor } : {}) }).toString()}`, 'GET', undefined, signal),
    derivedControlPreview: (scope: DerivedScope, channel: string, input: DerivedControlPreviewInput, signal?: AbortSignal) => call<DerivedControlPreview>(scope, `/channels/${id(channel)}/control-preview`, 'POST', input, signal),
    derivedControl: (scope: DerivedScope, channel: string, input: DerivedControlInput, signal?: AbortSignal) => call<Schema<'DerivedChannelResult'>>(scope, `/channels/${id(channel)}/control`, 'POST', input, signal),
    derivedTrial: (scope: DerivedScope, channel: string, input: DerivedTrialInput, signal?: AbortSignal) => call<DerivedRun>(scope, `/channels/${id(channel)}/trials`, 'POST', input, signal),
    derivedRuns: (scope: DerivedScope, filter: DerivedRunFilter = {}, signal?: AbortSignal) => call<Schema<'DerivedRunPage'>>(scope, `/runs${query(filter)}`, 'GET', undefined, signal),
    derivedRun: (scope: DerivedScope, run: string, signal?: AbortSignal) => call<DerivedRun>(scope, `/runs/${id(run)}`, 'GET', undefined, signal),
    derivedCancelRun: (scope: DerivedScope, run: string, input: Schema<'DerivedRunControlInput'>, signal?: AbortSignal) => call<DerivedRun>(scope, `/runs/${id(run)}/cancel`, 'POST', input, signal),
    derivedRetryRun: (scope: DerivedScope, run: string, input: Schema<'DerivedRunControlInput'>, signal?: AbortSignal) => call<DerivedRun>(scope, `/runs/${id(run)}/retry`, 'POST', input, signal),
    derivedChannels: (scope: DerivedScope, filter: DerivedChannelFilter = {}, signal?: AbortSignal) => call<Schema<'DerivedChannelPage'>>(scope, `/channels${query(filter)}`, 'GET', undefined, signal),
    derivedChannel: (scope: DerivedScope, channel: string, signal?: AbortSignal) => call<DerivedChannel>(scope, `/channels/${id(channel)}`, 'GET', undefined, signal),
    derivedCreateChannel: (scope: DerivedScope, input: DerivedCreateChannelInput, signal?: AbortSignal) => call<Schema<'DerivedChannelResult'>>(scope, '/channels', 'POST', input, signal),
    derivedChannelConfigs: (scope: DerivedScope, channel: string, filter: Pick<DerivedFilter, 'cursor' | 'limit'> = {}, signal?: AbortSignal) => call<Schema<'DerivedConfigPage'>>(scope, `/channels/${id(channel)}/configs${query(filter)}`, 'GET', undefined, signal),
    derivedDependencies: (scope: DerivedScope, channel: string, signal?: AbortSignal) => call<Schema<'DerivedDependencyPage'>>(scope, `/channels/${id(channel)}/dependencies`, 'GET', undefined, signal),
    derivedTemplates: (scope: DerivedScope, signal?: AbortSignal) => call<DerivedTemplatePage>(scope, '/templates', 'GET', undefined, signal),
    derivedExpandTemplate: (scope: DerivedScope, template: string, input: Schema<'DerivedExpandTemplateInput'>, signal?: AbortSignal) => call<DerivedRule>(scope, `/templates/${id(template)}/expand`, 'POST', input, signal),
    derivedMetrics: (scope: DerivedScope, filter: DerivedFilter = {}, signal?: AbortSignal) => call<DerivedMetricPage>(scope, `/metric-definitions${query(filter)}`, 'GET', undefined, signal),
    derivedMetric: (scope: DerivedScope, definition: string, signal?: AbortSignal) => call<DerivedMetricDefinition>(scope, `/metric-definitions/${id(definition)}`, 'GET', undefined, signal),
    derivedMetricVersions: (scope: DerivedScope, definition: string, filter: Pick<DerivedFilter, 'cursor' | 'limit'> = {}, signal?: AbortSignal) => call<DerivedVersionPage>(scope, `/metric-definitions/${id(definition)}/versions${query(filter)}`, 'GET', undefined, signal),
    derivedMetricVersion: (scope: DerivedScope, version: string, signal?: AbortSignal) => call<DerivedMetricVersion>(scope, `/metric-versions/${id(version)}`, 'GET', undefined, signal),
    derivedCreateMetric: (scope: TenantScope, input: DerivedCreateMetricInput, signal?: AbortSignal) => call<DerivedMetricResult>(scope, '/metric-definitions', 'POST', input, signal),
    derivedSaveMetric: (scope: TenantScope, definition: string, input: DerivedSaveMetricInput, signal?: AbortSignal) => call<DerivedMetricResult>(scope, `/metric-definitions/${id(definition)}/draft`, 'PATCH', input, signal),
    derivedCopyMetric: (scope: TenantScope, definition: string, input: Schema<'DerivedCopyMetricInput'>, signal?: AbortSignal) => call<DerivedMetricResult>(scope, `/metric-definitions/${id(definition)}/copy`, 'POST', input, signal),
    derivedValidateMetric: (scope: TenantScope, definition: string, input: Schema<'DerivedValidateInput'>, signal?: AbortSignal) => call<DerivedRuleValidation>(scope, `/metric-definitions/${id(definition)}/validate`, 'POST', input, signal),
    derivedPublishMetric: (scope: TenantScope, definition: string, input: DerivedPublishMetricInput, signal?: AbortSignal) => call<DerivedMetricResult>(scope, `/metric-definitions/${id(definition)}/publish`, 'POST', input, signal),
    derivedSetVersionState: (scope: TenantScope, version: string, input: Schema<'DerivedVersionStateInput'>, signal?: AbortSignal) => call<DerivedMetricResult>(scope, `/metric-versions/${id(version)}/state`, 'POST', input, signal),
    derivedDeleteMetricDraft: (scope: TenantScope, definition: string, input: Schema<'DerivedDeleteDraftInput'>, signal?: AbortSignal) => call<DerivedMetricResult>(scope, `/metric-definitions/${id(definition)}/draft`, 'DELETE', input, signal),
  };
}
