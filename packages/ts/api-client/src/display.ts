import type { MonitoringRequestOptions } from "./monitoring";
export type DisplayScope = { tenantId: string; projectId: string };
export type DisplaySchema = {
  type: "object" | "string" | "number" | "integer" | "boolean" | "array";
  title?: string;
  description?: string;
  default?: unknown;
  properties?: Record<string, DisplaySchema>;
  required?: string[];
  enum?: unknown[];
  maxLength?: number;
  minLength?: number;
  minimum?: number;
  maximum?: number;
  items?: DisplaySchema;
  maxItems?: number;
};
export type DisplayBinding = {
  kind: "channels" | "resources" | "spatial" | "artifacts";
  ids: string[];
  from?: string;
  to?: string;
  dynamic?: {
    facility_ids: string[];
    component_ids?: string[];
    include_future: boolean;
    metric_version_id?: string;
    value_kind?: string;
    source_kind?: string;
    baseline_ids?: string[];
  };
};
export type DisplayManifest = {
  app_key: string;
  name?: string;
  version: string;
  required_capabilities: Record<string, number>;
  optional_capabilities?: Record<string, number>;
  config_schema: DisplaySchema;
  binding_schema: Record<
    string,
    {
      kind: DisplayBinding["kind"];
      required: boolean;
      min_items?: number;
      max_items?: number;
      value_kind?: string;
      source_kind?: string;
      unit_dimension?: string;
    }
  >;
  files: { path: string; size: number; sha256: string }[];
};
export type DisplayInstance = {
  id: string;
  name: string;
  state: "enabled" | "disabled" | "archived";
  revision: string;
  current_release_id: string;
  draft_config_id: string;
  viewers: string[];
};
export type DisplayPackage = {
  id: string;
  sha256: string;
  state: string;
  manifest: DisplayManifest;
  created_at: string;
};
export type DisplayConfig = {
  id: string;
  package_id: string;
  parameters: Record<string, unknown>;
  bindings: Record<string, DisplayBinding>;
  mode: "fixed" | "follow";
  created_at: string;
};
export type DisplayGrant = {
  revision?: string;
  capabilities: string[];
  scope: {
    channels: string[];
    resources: string[];
    spatial: string[];
    artifacts: string[];
    dynamic?: DisplayBinding["dynamic"];
    from?: string;
    to?: string;
  };
  fields: string[];
  valid_until: string;
  revoked: boolean;
};
export type DisplayCapability = {
  name: string;
  availability: "available" | "unavailable";
  reason?: string;
  version: number;
};
export type DisplayRelease = {
  id: string;
  package_id: string;
  config_id: string;
  created_at: string;
  withdrawn: boolean;
};
export type DisplayUpload = {
  id: string;
  size: string;
  state: string;
  job?: {
    id: string;
    state: string;
    issues?: { path: string; message: string }[];
    error_code: string;
  };
};
export type DisplayDetail = {
  instance: DisplayInstance;
  access_url: string;
  packages?: DisplayPackage[];
  configs?: DisplayConfig[];
  grant?: DisplayGrant;
  releases?: DisplayRelease[];
  uploads?: DisplayUpload[];
  sessions?: {
    id: string;
    release_id: string;
    preview: boolean;
    state: string;
    last_seen_at: string;
    last_data_at?: string;
    last_query_state?: string;
    last_error: string;
  }[];
  terminals?: {
    id: string;
    name: string;
    state: string;
    generation: string;
    actual_release_id: string;
    valid_until: string;
    last_seen_at: string;
    last_error: string;
  }[];
  events?: { id: string; kind: string; details: unknown; created_at: string }[];
};
export type DisplayList = {
  settings: { enabled: boolean; revision: string };
  items: DisplayInstance[];
  runtime_origin: string;
  capabilities: DisplayCapability[];
};
export type DisplayResult = {
  id: string;
  revision: string;
  config_id?: string;
  release_id?: string;
  upload_id?: string;
  pair_code?: string;
};
type Request = <T>(
  path: string,
  method?: string,
  body?: unknown,
  notify?: boolean,
  signal?: AbortSignal,
  options?: MonitoringRequestOptions,
) => Promise<T>;
export const displayRoot = (s: DisplayScope) =>
  `/tenants/${encodeURIComponent(s.tenantId)}/projects/${encodeURIComponent(s.projectId)}/displays`;
export function displayMethods(request: Request) {
  return {
    displayList: (s: DisplayScope, signal?: AbortSignal) =>
      request<DisplayList>(displayRoot(s), "GET", undefined, true, signal),
    displayDetail: (s: DisplayScope, id: string, signal?: AbortSignal) =>
      request<DisplayDetail>(
        `${displayRoot(s)}/instances/${encodeURIComponent(id)}`,
        "GET",
        undefined,
        true,
        signal,
      ),
    displayCommand: (
      s: DisplayScope,
      path: string,
      input: object,
      method = "POST",
      signal?: AbortSignal,
    ) =>
      request<DisplayResult>(
        `${displayRoot(s)}${path}`,
        method,
        input,
        true,
        signal,
        { timeoutMs: 30000 },
      ),
    displayPrepare: (
      s: DisplayScope,
      id: string,
      config_id: string,
      expected_revision: string,
    ) =>
      request<{
        hash: string;
        binding_sets: Record<string, string[]>;
        grant_revision: string;
      }>(
        `${displayRoot(s)}/instances/${encodeURIComponent(id)}/prepare`,
        "POST",
        { config_id, expected_revision },
      ),
    displayLocate: (id: string) =>
      request<{ tenant_id: string; project_id: string }>(
        `/displays/${encodeURIComponent(id)}/location`,
      ),
    displayLaunch: (
      s: DisplayScope,
      id: string,
      action: "launch" | "preview" | "tool-authorize",
      input: object,
    ) =>
      request<{ code: string; runtime_origin: string }>(
        `${displayRoot(s)}/instances/${encodeURIComponent(id)}/${action}`,
        "POST",
        input,
      ),
    displayPart: (
      s: DisplayScope,
      id: string,
      upload: string,
      part: number,
      data: Blob,
      sha: string,
      signal?: AbortSignal,
    ) =>
      request<unknown>(
        `${displayRoot(s)}/instances/${encodeURIComponent(id)}/uploads/${encodeURIComponent(upload)}/parts/${part}`,
        "PUT",
        undefined,
        true,
        signal,
        {
          binary: data,
          headers: { "X-Content-SHA256": sha },
          timeoutMs: 120000,
        },
      ),
  };
}
