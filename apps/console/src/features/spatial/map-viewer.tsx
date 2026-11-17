import { useEffect, useRef, useState } from "react";
import Map from "ol/Map";
import View from "ol/View";
import Feature from "ol/Feature";
import Point from "ol/geom/Point";
import GeoJSON from "ol/format/GeoJSON";
import VectorSource from "ol/source/Vector";
import Cluster from "ol/source/Cluster";
import XYZ from "ol/source/XYZ";
import TileLayer from "ol/layer/Tile";
import VectorLayer from "ol/layer/Vector";
import { Style, Circle, Fill, Stroke, Text } from "ol/style";
import { fromLonLat, toLonLat, transformExtent } from "ol/proj";
import { createEmpty, extend, isEmpty } from "ol/extent";
import type {
  SpatialObject,
  SpatialFeature,
  SpatialObjectRef,
} from "@infrapro/api-client";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@infrapro/ui/components/button";
import { useSpatial } from "./context";
import { inputClass, sameObject } from "./state";
import { Failure } from "./common";
import "ol/ol.css";
type Props = {
  objects: SpatialObject[];
  features: SpatialFeature[];
  selected?: SpatialObjectRef;
  onSelect: (o: SpatialObjectRef) => void;
  onHover: (o?: SpatialObjectRef) => void;
  onBBox: (bbox: string) => void;
  basemap?: string;
  onBasemap?: (id: string) => void;
};
export default function MapViewer(props: Props) {
  const m = useSpatial(),
    host = useRef<HTMLDivElement>(null),
    mapRef = useRef<Map | null>(null),
    initialized = useRef(false),
    located = useRef(""),
    latest = useRef(props);
  useEffect(() => {
    latest.current = props;
  }, [props]);
  const pointSource = useRef(new VectorSource()),
    shapeSource = useRef(new VectorSource()),
    tileLayer = useRef(new TileLayer({ visible: false })),
    [tileFailed, setTileFailed] = useState(false),
    [base, setBase] = useState(props.basemap ?? "local");
  const basemaps = useQuery({
    queryKey: [...m.prefix, "basemaps"],
    queryFn: ({ signal }) => m.client.spatialBasemaps(m.scope, signal),
    enabled: m.available,
  });
  useEffect(() => {
    if (!host.current) return;
    const points = pointSource.current,
      shapesSource = shapeSource.current;
    const cluster = new Cluster({
      distance: 32,
      minDistance: 12,
      source: pointSource.current,
    });
    const vector = new VectorLayer({
      source: cluster,
      style: (f) => {
        const children: Feature[] = f.get("features") ?? [],
          o = children[0]?.get("owner"),
          selected =
            children.length === 1 && sameObject(latest.current.selected, o),
          retired = children.length === 1 && children[0]?.get("retired");
        return new Style({
          image: new Circle({
            radius: children.length > 1 ? 16 : selected ? 10 : 7,
            fill: new Fill({
              color: retired ? "#64748b" : selected ? "#c2410c" : "#0369a1",
            }),
            stroke: new Stroke({ color: "#fff", width: 2 }),
          }),
          text:
            children.length > 1
              ? new Text({
                  text: String(children.length),
                  fill: new Fill({ color: "#fff" }),
                })
              : undefined,
        });
      },
    });
    const shapes = new VectorLayer({
      source: shapeSource.current,
      style: (f) => {
        const category = String(f.get("category") ?? "");
        const palette: Record<string, [string, string]> = {
          boundary: ["#64748b", "rgba(100,116,139,.04)"],
          structure: ["#475569", "rgba(100,116,139,.25)"],
          alignment: ["#c56a22", "rgba(197,106,34,.12)"],
          water: ["#4697bc", "rgba(70,151,188,.28)"],
          contour: ["#b2997b", "rgba(178,153,123,.08)"],
          monitoring: ["#b85125", "rgba(184,81,37,.2)"],
        };
        const [line, fill] = palette[category] ?? [
          "#0369a1",
          "rgba(3,105,161,.12)",
        ];
        const selected =
          sameObject(latest.current.selected, f.get("owner")) &&
          (category === "boundary" || !category);
        return new Style({
          zIndex:
            category === "monitoring"
              ? 6
              : category === "alignment"
                ? 4
                : category === "structure"
                  ? 3
                  : 1,
          fill: new Fill({ color: fill }),
          stroke: new Stroke({
            color: selected ? "#c2410c" : line,
            width: selected ? 3 : category === "contour" ? 1 : 1.5,
            lineDash: category === "boundary" ? [7, 5] : undefined,
          }),
          image: new Circle({
            radius: 5,
            fill: new Fill({ color: f.get("retired") ? "#64748b" : line }),
            stroke: new Stroke({ color: "white", width: 1.5 }),
          }),
        });
      },
    });
    const map = new Map({
      target: host.current,
      layers: [tileLayer.current, shapes, vector],
      view: new View({ center: [0, 0], zoom: 2, minZoom: 1, maxZoom: 20 }),
      controls: [],
    });
    mapRef.current = map;
    const ro = new ResizeObserver(() => map.updateSize());
    ro.observe(host.current);
    let timer: ReturnType<typeof setTimeout> | undefined;
    map.on("moveend", () => {
      if (!initialized.current) return;
      clearTimeout(timer);
      timer = setTimeout(() => {
        const ext = map.getView().calculateExtent(),
          lower = toLonLat([ext[0]!, ext[1]!]),
          upper = toLonLat([ext[2]!, ext[3]!]),
          normalize = (n: number) => ((((n + 180) % 360) + 360) % 360) - 180;
        const world = ext[2]! - ext[0]! >= 40075016;
        latest.current.onBBox(
          [
            world ? -180 : normalize(lower[0]!),
            Math.max(-85.05112878, lower[1]!),
            world ? 180 : normalize(upper[0]!),
            Math.min(85.05112878, upper[1]!),
          ].join(","),
        );
      }, 250);
    });
    const userMove = () => {
      initialized.current = true;
    };
    map.on("pointerdrag", userMove);
    map.getViewport()?.addEventListener("wheel", userMove, { passive: true });
    map.on("singleclick", (e) => {
      const f = map.forEachFeatureAtPixel(e.pixel, (f) => f, {
        hitTolerance: 7,
      });
      const children: Feature[] = f?.get("features") ?? [];
      if (children.length > 1) {
        const extent = new VectorSource({ features: children }).getExtent();
        if (extent)
          map.getView().fit(extent, {
            padding: [80, 80, 80, 80],
            maxZoom: 18,
            duration: 0,
          });
        return;
      }
      const owner = children[0]?.get("owner") ?? f?.get("owner");
      if (owner) latest.current.onSelect(owner);
    });
    map.on("pointermove", (e) => {
      if (e.dragging) return;
      const f = map.forEachFeatureAtPixel(e.pixel, (f) => f, {
        hitTolerance: 7,
      });
      const children: Feature[] = f?.get("features") ?? [];
      latest.current.onHover(
        children.length === 1 ? children[0]?.get("owner") : f?.get("owner"),
      );
    });
    return () => {
      clearTimeout(timer);
      map.getViewport()?.removeEventListener("wheel", userMove);
      ro.disconnect();
      map.setTarget(undefined);
      map.dispose();
      mapRef.current = null;
      initialized.current = false;
      located.current = "";
      points.clear();
      shapesSource.clear();
    };
  }, []);
  useEffect(() => {
    pointSource.current.clear();
    for (const o of props.objects) {
      if (
        o.longitude === undefined ||
        o.latitude === undefined ||
        o.longitude === null ||
        o.latitude === null
      )
        continue;
      pointSource.current.addFeature(
        new Feature({
          geometry: new Point(fromLonLat([o.longitude, o.latitude])),
          owner: { kind: o.kind, id: o.id },
          retired: o.retired,
        }),
      );
    }
    shapeSource.current.clear();
    for (const f of props.features) {
      try {
        const feature = new GeoJSON().readFeature(
          { type: "Feature", geometry: f.geometry, properties: f.properties },
          { dataProjection: "EPSG:4326", featureProjection: "EPSG:3857" },
        );
        if (Array.isArray(feature)) continue;
        feature.set(
          "owner",
          f.facility ? { kind: "facility", id: f.facility.id } : undefined,
        );
        feature.set("retired", f.facility?.retired ?? false);
        shapeSource.current.addFeature(feature);
      } catch {
        /* A failed optional layer leaves the resource map usable. */
      }
    }
    if (!initialized.current) {
      const extent = createEmpty();
      const points = pointSource.current.getExtent(),
        shapes = shapeSource.current.getExtent();
      if (points) extend(extent, points);
      if (shapes) extend(extent, shapes);
      if (!isEmpty(extent)) {
        initialized.current = true;
        mapRef.current?.getView().fit(extent, {
          padding: [70, 70, 70, 70],
          maxZoom: 20,
        });
      }
    }
    mapRef.current?.render();
  }, [props.objects, props.features]);
  useEffect(() => {
    mapRef.current?.render();
    const selected = props.objects.find((o) => sameObject(o, props.selected));
    const feature = props.features.find(
      (f) => f.facility && sameObject(f.facility, props.selected),
    );
    const stamp = JSON.stringify([
      props.selected,
      selected?.longitude,
      selected?.latitude,
      feature?.bbox,
    ]);
    if (located.current === stamp) return;
    located.current = stamp;
    if (selected?.longitude != null && selected.latitude != null) {
      initialized.current = true;
      mapRef.current
        ?.getView()
        .setCenter(fromLonLat([selected.longitude, selected.latitude]));
    } else if (feature) {
      initialized.current = true;
      mapRef.current
        ?.getView()
        .fit(transformExtent(feature.bbox, "EPSG:4326", "EPSG:3857"), {
          padding: [80, 80, 80, 80],
          maxZoom: 20,
        });
    }
  }, [props.selected, props.objects, props.features]);
  useEffect(() => {
    const b = basemaps.data?.items.find((b) => b.id === base);
    tileLayer.current.setVisible(!!b?.url && !tileFailed);
    if (b?.url) {
      const source = new XYZ({
        url: b.url,
        attributions: b.attribution,
        minZoom: b.min_zoom,
        maxZoom: b.max_zoom,
        crossOrigin: "anonymous",
      });
      source.on("tileloaderror", () => setTileFailed(true));
      tileLayer.current.setSource(source);
    } else tileLayer.current.setSource(null);
  }, [basemaps.data, base, tileFailed]);
  const fit = () => {
    const extent = pointSource.current.getExtent();
    if (extent && !isEmpty(extent))
      mapRef.current
        ?.getView()
        .fit(extent, { padding: [70, 70, 70, 70], maxZoom: 17 });
    else {
      const extent = shapeSource.current.getExtent();
      if (extent && !isEmpty(extent))
        mapRef.current
          ?.getView()
          .fit(extent, { padding: [60, 60, 60, 60], maxZoom: 20 });
    }
  };
  const selectedExtent = props.features.find(
    (f) => f.facility && sameObject(f.facility, props.selected),
  )?.bbox;
  return (
    <div className="relative h-full min-h-96">
      <div
        ref={host}
        tabIndex={0}
        role="region"
        aria-label="GIS 地图，聚合标记可点击展开；资源目录提供键盘选择"
        className="h-full min-h-96 bg-slate-100"
      />
      <div className="absolute left-3 right-3 top-3 flex flex-wrap gap-2">
        <Button size="sm" variant="secondary" onClick={fit}>
          显示已载入位置
        </Button>
        {selectedExtent && (
          <Button
            size="sm"
            variant="secondary"
            onClick={() =>
              mapRef.current
                ?.getView()
                .fit(
                  transformExtent(selectedExtent, "EPSG:4326", "EPSG:3857"),
                  { padding: [80, 80, 80, 80], maxZoom: 20 },
                )
            }
          >
            定位所选图形
          </Button>
        )}
        <label className="flex items-center gap-2 rounded bg-background/95 px-2 text-xs">
          底图
          <select
            aria-label="地图底图"
            className={`${inputClass} !w-auto`}
            value={base}
            onChange={(e) => {
              setBase(e.target.value);
              setTileFailed(false);
              props.onBasemap?.(e.target.value);
            }}
          >
            {basemaps.data?.items.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className="absolute bottom-3 left-3 right-3 rounded bg-background/95 p-2 text-xs text-muted-foreground">
        {tileFailed ? "底图暂不可用，已使用空白背景。 " : ""}
        有效地理位置直接来自资源；没有坐标的对象保留在目录。本地底图为合成示意。
      </p>
      {basemaps.error && <Failure error={basemaps.error} />}
    </div>
  );
}
