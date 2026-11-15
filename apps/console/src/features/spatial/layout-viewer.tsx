import { useEffect, useRef, useState } from "react";
import Map from "ol/Map";
import View from "ol/View";
import Projection from "ol/proj/Projection";
import ImageLayer from "ol/layer/Image";
import ImageStatic from "ol/source/ImageStatic";
import VectorLayer from "ol/layer/Vector";
import VectorSource from "ol/source/Vector";
import Feature from "ol/Feature";
import Point from "ol/geom/Point";
import Polygon from "ol/geom/Polygon";
import { Style, Circle, Fill, Stroke, Text } from "ol/style";
import Translate from "ol/interaction/Translate";
import Draw from "ol/interaction/Draw";
import type { SpatialPage } from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { Failure, useBlobURL, useSpatialFile } from "./common";
import { sameObject, type SceneProps } from "./state";
import "ol/ol.css";
export default function LayoutViewer({
  page,
  version,
  ...props
}: SceneProps & { page: SpatialPage; version: string }) {
  const [fileID, setFileID] = useState(
      page.resolutions[0]?.file_id ?? page.file_id,
    ),
    imageLayer = useRef(new ImageLayer<ImageStatic>()),
    file = useSpatialFile(version, fileID),
    url = useBlobURL(file.data),
    host = useRef<HTMLDivElement>(null),
    mapRef = useRef<Map | null>(null),
    latest = useRef(props);
  useEffect(() => {
    latest.current = props;
  }, [props]);
  const source = useRef(new VectorSource()),
    focused = useRef("");
  useEffect(() => {
    if (!host.current) return;
    const markers = source.current;
    const extent = [0, 0, page.width, page.height],
      projection = new Projection({
        code: `spatial-${version}-${page.id}`,
        units: "pixels",
        extent,
      });
    const map = new Map({
      target: host.current,
      layers: [imageLayer.current, new VectorLayer({ source: source.current })],
      view: new View({
        projection,
        center: [page.width / 2, page.height / 2],
        zoom: 1,
        minZoom: -4,
        maxZoom: 12,
      }),
      controls: [],
    });
    mapRef.current = map;
    const resolution = () => {
      const desired =
        (page.width / (map.getView().getResolution() ?? 1)) *
        Math.min(window.devicePixelRatio || 1, 2);
      setFileID(
        [...page.resolutions]
          .sort((a, b) => a.width - b.width)
          .find((r) => r.width >= desired)?.file_id ?? page.file_id,
      );
    };
    map.getView().on("change:resolution", resolution);
    const fit = () =>
      map.getView().fit(extent, { padding: [40, 40, 40, 40], duration: 0 });
    fit();
    const observer = new ResizeObserver(() => {
      map.updateSize();
      resolution();
    });
    observer.observe(host.current);
    map.on("singleclick", (e) => {
      const feature = map.forEachFeatureAtPixel(e.pixel, (f) => f, {
        hitTolerance: 8,
      });
      const p = latest.current;
      if (feature) {
        const owner = feature.get("owner");
        p.onPick({ owner, page: page.id });
      } else if (p.editing && p.placing === "point") {
        p.onPick({
          anchor: [
            e.coordinate[0]! / page.width,
            1 - e.coordinate[1]! / page.height,
          ],
          page: page.id,
        });
      }
    });
    map.on("pointermove", (e) => {
      if (e.dragging) return;
      const feature = map.forEachFeatureAtPixel(e.pixel, (f) => f, {
        hitTolerance: 6,
      });
      latest.current.onHover(feature?.get("owner"));
      if (host.current)
        host.current.style.cursor = feature
          ? "pointer"
          : latest.current.editing
            ? "crosshair"
            : "grab";
    });
    map
      .getViewport()
      ?.addEventListener("pointerleave", () =>
        latest.current.onHover(undefined),
      );
    return () => {
      observer.disconnect();
      map.setTarget(undefined);
      map.dispose();
      mapRef.current = null;
      markers.clear();
    };
  }, [
    version,
    page.id,
    page.width,
    page.height,
    page.resolutions,
    page.file_id,
  ]);
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !url) return;
    imageLayer.current.setSource(
      new ImageStatic({
        url,
        projection: map.getView().getProjection(),
        imageExtent: [0, 0, page.width, page.height],
      }),
    );
  }, [url, page.width, page.height]);
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const render = () => {
      const resolution = map.getView().getResolution() ?? 1;
      source.current.clear();
      for (const p of props.placements) {
        if (p.page_id !== page.id) continue;
        const x = p.anchor[0]! * page.width,
          y = (1 - p.anchor[1]!) * page.height,
          selected = sameObject(props.selected, {
            kind: "point",
            id: p.point_id,
          }),
          retired = p.object?.retired;
        const feature = new Feature({
          geometry: new Point([x, y]),
          owner: { kind: "point", id: p.point_id },
          placement: p,
          part: "anchor",
        });
        feature.setStyle(
          new Style({
            image: new Circle({
              radius: selected ? 9 : 6,
              fill: new Fill({
                color: retired ? "#64748b" : selected ? "#c2410c" : "#0369a1",
              }),
              stroke: new Stroke({ color: "#fff", width: 2 }),
            }),
          }),
        );
        source.current.addFeature(feature);
        const label = new Feature({
          geometry: new Point([
            x + (p.label_offset[0] ?? 12) * resolution,
            y - (p.label_offset[1] ?? -12) * resolution,
          ]),
          owner: { kind: "point", id: p.point_id },
          placement: p,
          part: "label",
        });
        label.setStyle(
          new Style({
            text: new Text({
              text: `${retired ? "已退役 · " : ""}${p.object?.name ?? p.point_id.slice(0, 8)}${p.basis_changed ? " · 依据变化" : ""}`,
              font: "12px system-ui",
              textAlign: "left",
              fill: new Fill({ color: "#0f172a" }),
              backgroundFill: new Fill({ color: "rgba(255,255,255,.92)" }),
              padding: [4, 6, 4, 6],
            }),
          }),
        );
        source.current.addFeature(label);
      }
      for (const l of props.links) {
        if (l.page_id !== page.id || !l.region.length) continue;
        const ring = l.region.map((p) => [
          p[0]! * page.width,
          (1 - p[1]!) * page.height,
        ]);
        const feature = new Feature({
          geometry: new Polygon([ring]),
          owner: { kind: "component", id: l.component_id },
        });
        const selected = sameObject(props.selected, {
          kind: "component",
          id: l.component_id,
        });
        feature.setStyle(
          new Style({
            fill: new Fill({
              color: selected ? "rgba(194,65,12,.22)" : "rgba(3,105,161,.08)",
            }),
            stroke: new Stroke({
              color: selected ? "#c2410c" : "#0369a1",
              width: selected ? 3 : 1.5,
              lineDash: [6, 4],
            }),
          }),
        );
        source.current.addFeature(feature);
      }
    };
    render();
    map.getView().on("change:resolution", render);
    return () => map.getView().un("change:resolution", render);
  }, [url, page, props.placements, props.links, props.selected]);
  useEffect(() => {
    const map = mapRef.current,
      key = props.selected
        ? `${props.selected.kind}:${props.selected.id}:${page.id}`
        : "";
    if (!map || !key || focused.current === key || props.editing) return;
    const point = props.placements.find(
        (p) => p.point_id === props.selected?.id && p.page_id === page.id,
      ),
      link = props.links.find(
        (l) => l.component_id === props.selected?.id && l.page_id === page.id,
      );
    if (point) {
      map
        .getView()
        .setCenter([
          point.anchor[0]! * page.width,
          (1 - point.anchor[1]!) * page.height,
        ]);
      focused.current = key;
    } else if (link?.region.length) {
      const extent = new Polygon([
        link.region.map((p) => [p[0]! * page.width, (1 - p[1]!) * page.height]),
      ]).getExtent();
      map.getView().fit(extent, { padding: [80, 80, 80, 80], maxZoom: 4 });
      focused.current = key;
    }
  }, [props.selected, props.placements, props.links, props.editing, page]);
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !props.editing) return;
    const translate = new Translate({
      filter: (f) => !!f.get("placement"),
      hitTolerance: 8,
    });
    translate.on("translateend", (e) => {
      const f = e.features.item(0),
        p = f?.get("placement");
      if (!p) return;
      const coord = (f.getGeometry() as Point).getCoordinates(),
        anchor = [...p.anchor],
        label = [...p.label_offset],
        res = map.getView().getResolution() ?? 1;
      if (f.get("part") === "label") {
        label[0] = Math.round((coord[0]! - anchor[0] * page.width) / res);
        label[1] = Math.round(
          ((1 - anchor[1]) * page.height - coord[1]!) / res,
        );
      } else {
        anchor[0] = Math.max(0, Math.min(1, coord[0]! / page.width));
        anchor[1] = Math.max(0, Math.min(1, 1 - coord[1]! / page.height));
      }
      latest.current.onMove?.(p.id, anchor, label);
    });
    map.addInteraction(translate);
    const draw =
      props.placing === "component"
        ? new Draw({ type: "Polygon", maxPoints: 64 })
        : undefined;
    if (draw) {
      draw.on("drawend", (e) => {
        const coords = (e.feature.getGeometry() as Polygon)
          .getCoordinates()[0]!
          .map((p) => [p[0]! / page.width, 1 - p[1]! / page.height]);
        latest.current.onPick({ region: coords, page: page.id });
      });
      map.addInteraction(draw);
    }
    return () => {
      map.removeInteraction(translate);
      if (draw) map.removeInteraction(draw);
    };
  }, [url, page, props.editing, props.placing]);
  return (
    <div className="relative h-full min-h-96">
      <div
        ref={host}
        tabIndex={0}
        role="region"
        aria-label={`布设图第 ${page.number} 页，可用加减键缩放和方向键平移`}
        className="h-full min-h-96 bg-slate-100"
      />
      <div className="absolute left-3 top-3 flex gap-2">
        <Button
          size="sm"
          variant="secondary"
          onClick={() =>
            mapRef.current?.getView().fit([0, 0, page.width, page.height], {
              padding: [40, 40, 40, 40],
            })
          }
        >
          适配图纸
        </Button>
        <Button
          size="sm"
          variant="secondary"
          aria-label="放大布设图"
          onClick={() => {
            const v = mapRef.current?.getView();
            v?.setZoom((v.getZoom() ?? 0) + 1);
          }}
        >
          ＋
        </Button>
        <Button
          size="sm"
          variant="secondary"
          aria-label="缩小布设图"
          onClick={() => {
            const v = mapRef.current?.getView();
            v?.setZoom((v.getZoom() ?? 0) - 1);
          }}
        >
          −
        </Button>
      </div>
      {file.isPending && (
        <p
          role="status"
          className="absolute bottom-3 left-3 rounded bg-background p-2"
        >
          正在读取图纸…
        </p>
      )}
      {file.error && (
        <div className="absolute inset-x-3 bottom-3">
          <Failure error={file.error} />
        </div>
      )}
    </div>
  );
}
