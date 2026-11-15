import { DisplayError } from "./error.js";
import type {
  DisplayTransport,
  RuntimeEvent,
  SubscriptionEvent,
} from "./types.js";
export type FixtureScenario =
  | "normal"
  | "empty"
  | "pending"
  | "disconnected"
  | "revoked"
  | "unavailable";
/** Explicitly simulated data; never used automatically when a real request fails. */
export function createFixtureTransport(
  scenario: FixtureScenario = "normal",
): DisplayTransport & { emit(event: SubscriptionEvent | RuntimeEvent): void } {
  const listeners = new Set<
    (event: SubscriptionEvent | RuntimeEvent) => void
  >();
  let closed = false,
    serial = 0;
  const emit = (e: SubscriptionEvent | RuntimeEvent) => {
    for (const f of listeners) f(e);
  };
  return {
    emit,
    onEvent(f) {
      listeners.add(f);
      return () => {
        listeners.delete(f);
      };
    },
    dispose() {
      closed = true;
      listeners.clear();
    },
    async request<T>(method: string, params: unknown): Promise<T> {
      if (closed) throw new DisplayError("disposed");
      if (scenario === "revoked") {
        queueMicrotask(() => emit({ type: "revoked" }));
        throw new DisplayError("session_revoked");
      }
      if (scenario === "disconnected") {
        queueMicrotask(() => emit({ type: "disconnected" }));
        throw new DisplayError("dependency_unavailable");
      }
      if (method.startsWith("alerts.") || scenario === "unavailable")
        throw new DisplayError("capability_unavailable");
      let value: unknown = {};
      if (method === "context.get")
        value = {
          tenant_id: "mock-tenant",
          project_id: "mock-project",
          instance_id: "mock-instance",
          release_id: "mock-release",
          generation: "1",
          environment: "mock",
          preview: true,
          mode: "fixed",
          timezone: "Asia/Shanghai",
          server_time: "2026-11-13T00:00:00.000000001Z",
          simulated: true,
        };
      else if (method === "config.get") value = { title: "模拟 SDK 接入" };
      else if (method === "capabilities.list")
        value = [
          {
            name: "resources.read",
            version: 1,
            availability: "available",
            required_permissions: [],
            subscribe: false,
            max_targets: 500,
          },
        ];
      else if (method === "bindings.list")
        value = {site: {kind: "resources", ids:["mock-resource"], revision:"1", state:"ready"}};
      else if (method === "bindings.resolve")
        value = {
          kind: "resources",
          ids: ["mock-resource"],
          revision: "1",
          state: "ready",
        };
      else if (method === "subscription.create") {
        const id = `mock-sub-${++serial}`;
        value = { id };
        setTimeout(
          () =>
            emit({
              subscription_id: id,
              generation: "1",
              sequence: "1",
              kind: scenario === "pending" ? "pending" : "snapshot",
              result: {
                state: scenario === "pending" ? "pending" : "ready",
                data: {
                  state: "ready",
                  selection_id: "mock-selection",
                  snapshot: {
                    id: "mock-snapshot",
                    watermark: "1",
                    route_revision: "1",
                    expires_at: "2099-01-01T00:00:00Z",
                    query_hash: "fixture",
                  },
                  channels: [],
                  computation: "fixture",
                  has_more: false,
                  filters: {
                    all_history: false,
                    qualities: ["good"],
                    timezone: "Asia/Shanghai",
                  },
                },
                generation: "1",
                authorization_revision: "1",
                read_at: "2026-11-13T00:00:00.000000001Z",
                source_vector: { fixture: "1" },
              },
            }),
          0,
        );
      } else if (
        !method.startsWith("runtime.") &&
        !method.startsWith("subscription.") &&
        !method.startsWith("navigation.")
      )
        value = {
          state:
            scenario === "pending"
              ? "pending"
              : scenario === "empty"
                ? "empty"
                : "ready",
          data: {
            items:
              scenario === "empty"
                ? []
                : [
                    {
                      id: "mock-resource",
                      kind: "facility",
                      name: "模拟设施",
                      code: "MOCK",
                      revision: "1",
                      value: "9007199254740993.000000001",
                    },
                  ],
            count: scenario === "empty" ? "0" : "1",
            next_cursor: "",
          },
          read_at: "2026-11-13T00:00:00.000000001Z",
          authorization_revision: "1",
          generation: "1",
          source_vector: { fixture: "1" },
        };
      void params;
      return value as T;
    },
  };
}
