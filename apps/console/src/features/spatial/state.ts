import type {
  SpatialObjectRef,
  SpatialPlacement,
  SpatialComponentLink,
  SpatialSettings,
} from "@infrapro/api-client";
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;
export const spatialSections = [
  "overview",
  "gis",
  "layouts",
  "models",
  "placements",
  "assets",
] as const;
export type SpatialSection = (typeof spatialSections)[number];
export const sectionForKind = (kind: string): SpatialSection =>
  kind === "layout" ? "layouts" : kind === "model" ? "models" : "gis";
export type SpatialSearch = {
  asset?: string;
  version?: string;
  owner?: string;
  owner_kind?: string;
  facility?: string;
  page?: string;
  view?: string;
  mode?: string;
  section?: SpatialSection;
  q?: string;
  kind?: string;
  status?: string;
};
export function spatialSearch(s: Record<string, unknown>): SpatialSearch {
  const id = (k: string) =>
    typeof s[k] === "string" && uuid.test(s[k]) ? s[k] : undefined;
  return {
    asset: id("asset"),
    version: id("version"),
    owner: id("owner"),
    owner_kind: ["facility", "component", "point"].includes(
      String(s.owner_kind),
    )
      ? String(s.owner_kind)
      : undefined,
    facility: id("facility"),
    page:
      typeof s.page === "string" && s.page.length < 160 ? s.page : undefined,
    view: id("view"),
    mode: ["manage", "edit"].includes(String(s.mode))
      ? String(s.mode)
      : undefined,
    section: spatialSections.includes(s.section as SpatialSection)
      ? (s.section as SpatialSection)
      : undefined,
    q: typeof s.q === "string" ? s.q.slice(0, 200) : undefined,
    kind: ["layout", "model", "geolayer"].includes(String(s.kind))
      ? String(s.kind)
      : undefined,
    status: ["normal", "retired"].includes(String(s.status))
      ? String(s.status)
      : undefined,
  };
}
export type Pick = {
  owner?: SpatialObjectRef;
  element?: string;
  anchor?: number[];
  region?: number[][];
  page?: string;
};
export type SceneProps = {
  placements: SpatialPlacement[];
  links: SpatialComponentLink[];
  selected?: SpatialObjectRef;
  element?: string;
  settings: SpatialSettings;
  onPick: (p: Pick) => void;
  onHover: (p?: SpatialObjectRef) => void;
  editing?: boolean;
  placing?: "point" | "component";
  onMove?: (id: string, anchor: number[], label: number[]) => void;
  onHiddenElements?: (elements: string[]) => void;
  onCamera?: (camera: NonNullable<SpatialSettings["camera"]>) => void;
};
export const sameObject = (a?: SpatialObjectRef, b?: SpatialObjectRef) =>
  !!a && !!b && a.kind === b.kind && a.id === b.id;
export const objectKey = (o: SpatialObjectRef) => `${o.kind}:${o.id}`;
export const kinds: Record<string, string> = {
  facility: "设施",
  component: "构件",
  point: "测点",
  channel: "通道",
};
export const assetKinds: Record<string, string> = {
  layout: "布设图",
  model: "BIM / 三维模型",
  geolayer: "GIS 参考图层",
};
export const inputClass =
  "min-h-11 w-full rounded-md border bg-background px-3 py-2 text-sm disabled:opacity-50";
