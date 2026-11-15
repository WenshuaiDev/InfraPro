import type { SpatialPreview } from "@infrapro/api-client";
import { Failure, Info } from "./common";

export function PreviewFacts({ preview }: { preview: SpatialPreview }) {
  const changes =
    preview.changes && typeof preview.changes === "object"
      ? (preview.changes as Record<string, unknown>)
      : {};
  return (
    <div className="space-y-3">
      {["source_content", "view_scope", "source_scope", "business_access"].map(
        (key) =>
          typeof changes[key] === "string" ? (
            <Info key={key}>{changes[key] as string}</Info>
          ) : null,
      )}
      {typeof changes.unadopted_candidates === "number" &&
        changes.unadopted_candidates > 0 && (
          <Info>
            尚有 {changes.unadopted_candidates}{" "}
            项未采用的迁移候选。发布前请确认处理结果。
          </Info>
        )}
      {preview.blockers.map((b, i) => (
        <Failure key={i} error={b.message} />
      ))}
      {preview.visible_impacts.length > 0 && (
        <section className="space-y-2">
          <h4 className="text-sm font-medium">可见的关联影响</h4>
          {preview.visible_impacts.map((impact, i) => (
            <pre
              key={i}
              className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded border p-3 text-xs"
            >
              {typeof impact === "string"
                ? impact
                : JSON.stringify(impact, null, 2)}
            </pre>
          ))}
        </section>
      )}
      <details>
        <summary className="cursor-pointer py-2 text-xs text-muted-foreground">
          查看完整预览字段
        </summary>
        <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all rounded bg-muted p-3 text-xs">
          {JSON.stringify(
            {
              changes: preview.changes,
              visible_impacts: preview.visible_impacts,
            },
            null,
            2,
          )}
        </pre>
      </details>
    </div>
  );
}
