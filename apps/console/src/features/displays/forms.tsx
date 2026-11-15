import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useIdentity } from "@infrapro/api-client/react";
import type {
  DisplaySchema,
  DisplayBinding,
  ResourceKind,
} from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { Input } from "@infrapro/ui/components/input";
import { useProject } from "../projects/context";
export const fieldClass =
  "min-h-10 w-full rounded-md border bg-background px-3 py-2 text-sm";
export function SchemaField({
  schema,
  value,
  onChange,
  label,
  path = "param",
}: {
  schema: DisplaySchema;
  value: unknown;
  onChange: (v: unknown) => void;
  label: string;
  path?: string;
}) {
  const title = schema.title ?? label;
  if (schema.type === "object") {
    const record = (value ?? {}) as Record<string, unknown>;
    return (
      <fieldset className="space-y-4 rounded-lg border p-4">
        <legend className="px-2 font-medium">{title}</legend>
        {Object.entries(schema.properties ?? {}).map(([key, s]) => (
          <SchemaField
            key={key}
            path={`${path}-${key}`}
            label={`${key}${schema.required?.includes(key) ? " *" : ""}`}
            schema={s}
            value={record[key] ?? s.default}
            onChange={(v) => onChange({ ...record, [key]: v })}
          />
        ))}
      </fieldset>
    );
  }
  if (schema.type === "array") {
    const values = Array.isArray(value) ? value : [];
    return (
      <fieldset className="space-y-2">
        <legend>{title}</legend>
        {values.map((v, i) => (
          <div key={i} className="flex items-start gap-2">
            <div className="grow">
              <SchemaField
                schema={schema.items!}
                value={v}
                label={`${title} ${i + 1}`}
                path={`${path}-${i}`}
                onChange={(next) =>
                  onChange(values.map((old, j) => (i === j ? next : old)))
                }
              />
            </div>
            <Button
              variant="outline"
              aria-label={`移除 ${title} ${i + 1}`}
              onClick={() => onChange(values.filter((_, j) => i !== j))}
            >
              移除
            </Button>
          </div>
        ))}
        <Button
          variant="outline"
          disabled={values.length >= (schema.maxItems ?? 100)}
          onClick={() => onChange([...values, schema.items?.default ?? ""])}
        >
          添加一项
        </Button>
      </fieldset>
    );
  }
  return (
    <div className="space-y-1.5">
      <label htmlFor={path} className="text-sm font-medium">
        {title}
      </label>
      {schema.enum ? (
        <select
          id={path}
          className={fieldClass}
          value={JSON.stringify(value) ?? ""}
          onChange={(e) => onChange(JSON.parse(e.target.value))}
        >
          <option value="" disabled>
            请选择
          </option>
          {schema.enum.map((v, i) => (
            <option key={i} value={JSON.stringify(v)}>
              {String(v)}
            </option>
          ))}
        </select>
      ) : schema.type === "boolean" ? (
        <input
          id={path}
          type="checkbox"
          className="ml-3 size-4"
          checked={value === true}
          onChange={(e) => onChange(e.target.checked)}
        />
      ) : (
        <Input
          id={path}
          value={value === undefined ? "" : String(value)}
          type={["integer", "number"].includes(schema.type) ? "number" : "text"}
          min={schema.minimum}
          max={schema.maximum}
          step={schema.type === "integer" ? 1 : "any"}
          minLength={schema.minLength}
          maxLength={schema.maxLength}
          onChange={(e) =>
            onChange(
              ["integer", "number"].includes(schema.type)
                ? e.target.value === ""
                  ? undefined
                  : Number(e.target.value)
                : e.target.value,
            )
          }
        />
      )}{" "}
      {schema.description && (
        <p className="text-xs text-muted-foreground">{schema.description}</p>
      )}
    </div>
  );
}
// eslint-disable-next-line react-refresh/only-export-components
export function defaults(schema: DisplaySchema): unknown {
  if (schema.default !== undefined) return schema.default;
  if (schema.type === "object")
    return Object.fromEntries(
      Object.entries(schema.properties ?? {})
        .map(([k, s]) => [k, defaults(s)])
        .filter(([, v]) => v !== undefined),
    );
  return undefined;
}
export function ObjectPicker({
  kind,
  ids,
  onChange,
  label,
}: {
  kind: DisplayBinding["kind"];
  ids: string[];
  onChange: (ids: string[]) => void;
  label: string;
}) {
  const { client } = useIdentity(),
    project = useProject();
  const scope = {
    kind: "project" as const,
    tenantId: project.project.tenant_id,
    projectId: project.project.id,
  };
  const [search, setSearch] = useState(""),
    [cursor, setCursor] = useState(""),
    [facility, setFacility] = useState(""),
    [asset, setAsset] = useState(""),
    [resourceKind, setResourceKind] = useState<ResourceKind>("facility");
  const state = useQuery({
    queryKey: [
      ...project.key,
      "display-picker",
      kind,
      search,
      cursor,
      facility,
      asset,
      resourceKind,
    ],
    enabled: project.available,
    queryFn: async ({ signal }) => {
      if (kind === "channels") {
        const r = await client.monitoringDirectory(
          scope,
          { search, cursor },
          signal,
        );
        return {
          items: r.items.map((v) => ({
            id: v.id,
            name: `${v.name} · ${v.code}`,
            next: "",
          })),
          next: r.next_cursor,
        };
      }
      if (kind === "resources") {
        const r = await client.resourceList(
          scope,
          { kind: resourceKind, facility: facility || undefined },
          { q: search, cursor, limit: 30 },
          signal,
        );
        return {
          items: r.items.map((v) => ({
            id: v.id,
            name: `${v.name} · ${v.kind}`,
            next: v.kind === "facility" ? v.id : "",
          })),
          next: r.next_cursor,
        };
      }
      if (kind === "artifacts") {
        const r = await client.analysisArtifacts(scope, { cursor }, signal);
        return {
          items: r.items.map((v) => ({
            id: v.version.id,
            name: `${v.artifact.name} · v${v.version.version_no}`,
            next: "",
          })),
          next: r.next_cursor,
        };
      }
      if (asset) {
        const r = await client.spatialVersions(
          scope,
          asset,
          { cursor, limit: 30 },
          signal,
        );
        return {
          items: r.items
            .filter((v) => v.state === "published")
            .map((v) => ({
              id: v.id,
              name: `${v.format} · v${v.number}`,
              next: "",
            })),
          next: r.next_cursor,
        };
      }
      const r = await client.spatialAssets(
        scope,
        { q: search, cursor, limit: 30 },
        signal,
      );
      return {
        items: r.items.map((v) => ({ id: v.id, name: v.name, next: v.id })),
        next: r.next_cursor,
      };
    },
    retry: false,
  });
  return (
    <fieldset className="space-y-3 rounded-lg border p-3">
      <legend className="px-2 text-sm font-medium">
        {label} · 已选 {ids.length}
      </legend>
      <div className="flex flex-wrap gap-2">
        <Input
          aria-label={`搜索${label}`}
          placeholder="搜索名称或编码"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setCursor("");
          }}
        />
        {kind === "resources" && facility && (
          <select
            aria-label="资源层级"
            className={fieldClass}
            value={resourceKind}
            onChange={(e) => {
              setResourceKind(e.target.value as ResourceKind);
              setCursor("");
            }}
          >
            <option value="facility">设施</option>
            <option value="component">构件</option>
            <option value="point">测点</option>
            <option value="channel">通道</option>
          </select>
        )}
        {(facility || asset) && (
          <Button
            variant="outline"
            onClick={() => {
              setFacility("");
              setAsset("");
              setResourceKind("facility");
              setCursor("");
            }}
          >
            返回目录
          </Button>
        )}
      </div>
      {state.error && (
        <p role="alert" className="text-sm text-destructive">
          {state.error.message}
        </p>
      )}
      <div className="max-h-52 space-y-1 overflow-auto">
        {state.data?.items.map((item) => (
          <div
            key={item.id}
            className="flex items-center gap-2 rounded px-2 py-1 hover:bg-muted"
          >
            <label className="flex min-h-9 grow items-center gap-2 text-sm">
              <input
                type="checkbox"
                disabled={kind === "spatial" && !asset}
                checked={ids.includes(item.id)}
                onChange={(e) =>
                  onChange(
                    e.target.checked
                      ? [...ids, item.id]
                      : ids.filter((id) => id !== item.id),
                  )
                }
              />
              {item.name}
            </label>
            {item.next && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  if (kind === "spatial") {
                    setAsset(item.next);
                  } else {
                    setFacility(item.next);
                  }
                  if (kind === "resources") setResourceKind("component");
                  setCursor("");
                }}
              >
                展开
              </Button>
            )}
          </div>
        ))}
        {state.data?.items.length === 0 && (
          <p className="p-2 text-sm text-muted-foreground">
            没有匹配的可选对象
          </p>
        )}
      </div>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={!cursor}
          onClick={() => setCursor("")}
        >
          首页
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!state.data?.next}
          onClick={() => setCursor(state.data?.next ?? "")}
        >
          下一页
        </Button>
      </div>
      {ids.length > 0 && (
        <details>
          <summary className="cursor-pointer text-xs text-muted-foreground">
            已选对象标识（可移除）
          </summary>
          <ul className="space-y-1 pt-2">
            {ids.map((id) => (
              <li
                key={id}
                className="flex items-center justify-between gap-2 break-all text-xs"
              >
                {id}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onChange(ids.filter((v) => v !== id))}
                >
                  移除
                </Button>
              </li>
            ))}
          </ul>
        </details>
      )}
    </fieldset>
  );
}
export function BindingEditor({
  value,
  onChange,
  label,
}: {
  value: DisplayBinding;
  onChange: (v: DisplayBinding) => void;
  label: string;
}) {
  return (
    <div className="space-y-3">
      <ObjectPicker
        kind={value.kind}
        label={label}
        ids={value.ids ?? []}
        onChange={(ids) => onChange({ ...value, ids })}
      />
      {value.kind === "channels" && (
        <details className="rounded-lg border p-3">
          <summary className="cursor-pointer text-sm">
            动态集合与读取时段
          </summary>
          <div className="mt-3 space-y-3">
            <label className="flex gap-2 text-sm">
              <input
                type="checkbox"
                checked={!!value.dynamic}
                onChange={(e) =>
                  onChange({
                    ...value,
                    dynamic: e.target.checked
                      ? { facility_ids: [], include_future: false }
                      : undefined,
                  })
                }
              />
              按设施和语义动态选择
            </label>
            {value.dynamic && (
              <>
                <ObjectPicker
                  kind="resources"
                  label="动态设施范围"
                  ids={value.dynamic.facility_ids}
                  onChange={(facility_ids) =>
                    onChange({
                      ...value,
                      dynamic: { ...value.dynamic!, facility_ids },
                    })
                  }
                />
                <label className="flex gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={value.dynamic.include_future}
                    onChange={(e) =>
                      onChange({
                        ...value,
                        dynamic: {
                          ...value.dynamic!,
                          include_future: e.target.checked,
                        },
                      })
                    }
                  />
                  纳入未来新增的匹配通道（还需读取授权允许）
                </label>
                <label className="block text-sm">
                  来源类型
                  <select
                    className={fieldClass}
                    value={value.dynamic.source_kind ?? ""}
                    onChange={(e) =>
                      onChange({
                        ...value,
                        dynamic: {
                          ...value.dynamic!,
                          source_kind: e.target.value || undefined,
                        },
                      })
                    }
                  >
                    <option value="">全部</option>
                    <option value="direct">直接通道</option>
                    <option value="derived">派生通道</option>
                  </select>
                </label>
                <label className="block text-sm">
                  数值类型
                  <select
                    className={fieldClass}
                    value={value.dynamic.value_kind ?? ""}
                    onChange={(e) =>
                      onChange({
                        ...value,
                        dynamic: {
                          ...value.dynamic!,
                          value_kind: e.target.value || undefined,
                        },
                      })
                    }
                  >
                    <option value="">全部</option>
                    <option value="integer">整数</option>
                    <option value="decimal">小数</option>
                    <option value="enum">枚举</option>
                  </select>
                </label>
              </>
            )}
            <label className="block text-sm">
              起点（UTC RFC3339，可含纳秒）
              <Input
                placeholder="2026-01-01T00:00:00Z"
                value={value.from ?? ""}
                onChange={(e) =>
                  onChange({ ...value, from: e.target.value || undefined })
                }
              />
            </label>
            <label className="block text-sm">
              终点（不含）
              <Input
                value={value.to ?? ""}
                onChange={(e) =>
                  onChange({ ...value, to: e.target.value || undefined })
                }
              />
            </label>
          </div>
        </details>
      )}
    </div>
  );
}
