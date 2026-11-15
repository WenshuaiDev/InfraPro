import type { DisplayBinding, DisplayConfig, DisplayPackage, DisplaySchema } from "@infrapro/api-client";

/** A declarative projection only: new packages never execute migration code. */
export function upgradeConfig(pack: DisplayPackage, prior?: Pick<DisplayConfig, "parameters" | "bindings" | "mode">) {
  const inherited: string[] = [], replaced: string[] = [], missing: string[] = [], removed: string[] = [];
  function project(schema: DisplaySchema, old: unknown, path: string, required: boolean): unknown {
    if (schema.type === "object") {
      const source = old && typeof old === "object" && !Array.isArray(old) ? old as Record<string, unknown> : {};
      const result: Record<string, unknown> = {};
      for (const key of Object.keys(source)) if (!schema.properties?.[key]) removed.push(`${path}.${key}`);
      for (const [key, child] of Object.entries(schema.properties ?? {})) {
        const value = project(child, source[key], `${path}.${key}`, schema.required?.includes(key) ?? false);
        if (value !== undefined) result[key] = value;
      }
      return result;
    }
    function valid(s: DisplaySchema, value: unknown): boolean {
      if (s.enum && !s.enum.some(v => JSON.stringify(v) === JSON.stringify(value))) return false;
      switch (s.type) {
        case "string": return typeof value === "string" && value.length >= (s.minLength ?? 0) && value.length <= (s.maxLength ?? Infinity);
        case "integer": case "number": return typeof value === "number" && Number.isFinite(value) && (s.type !== "integer" || Number.isSafeInteger(value)) && value >= (s.minimum ?? -Infinity) && value <= (s.maximum ?? Infinity);
        case "boolean": return typeof value === "boolean";
        case "array": return Array.isArray(value) && value.length <= (s.maxItems ?? 1000) && !!s.items && value.every(v => valid(s.items!, v));
        default: return false;
      }
    }
    if (old !== undefined && valid(schema, old)) { inherited.push(path); return structuredClone(old); }
    if (old !== undefined) replaced.push(path);
    if (schema.default !== undefined) return structuredClone(schema.default);
    if (required) missing.push(path);
    return undefined;
  }
  const parameters = project(pack.manifest.config_schema, prior?.parameters, "参数", true) as Record<string, unknown>;
  const bindings: Record<string, DisplayBinding> = {};
  for (const key of Object.keys(prior?.bindings ?? {})) if (!pack.manifest.binding_schema[key]) removed.push(`绑定.${key}`);
  for (const [key, schema] of Object.entries(pack.manifest.binding_schema)) {
    const old = prior?.bindings[key], path = `绑定.${key}`;
    if (old?.kind === schema.kind && (old.dynamic || old.ids.length >= (schema.min_items ?? (schema.required ? 1 : 0))) && old.ids.length <= (schema.max_items ?? 500)) {
      bindings[key] = structuredClone(old); inherited.push(path);
    } else { bindings[key] = { kind: schema.kind, ids: [] }; if (old) replaced.push(path); if (schema.required) missing.push(path); }
  }
  return { parameters, bindings, mode: prior?.mode ?? "fixed", inherited, replaced, missing, removed };
}
