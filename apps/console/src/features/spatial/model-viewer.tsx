import { useEffect, useRef, useState } from "react";
import * as THREE from "three";
import { GLTFLoader } from "three/addons/loaders/GLTFLoader.js";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import type { SpatialElement } from "@infrapro/api-client";
import { Button } from "@infrapro/ui/components/button";
import { Failure, useSpatialFile } from "./common";
import { sameObject, placementRef, canPlace, type SceneProps } from "./state";
type Controller = {
  camera: (value: NonNullable<SceneProps["settings"]["camera"]>) => void;
  fit: (direction?: string) => void;
  apply: () => void;
  capture: () => void;
};
export default function ModelViewer({
  version,
  fileID,
  elements,
  ...props
}: SceneProps & {
  version: string;
  fileID?: string;
  elements: SpatialElement[];
}) {
  const file = useSpatialFile(version, fileID),
    host = useRef<HTMLDivElement>(null),
    api = useRef<Controller | null>(null),
    latest = useRef({ props, elements });
  useEffect(() => {
    latest.current = { props, elements };
  }, [props, elements]);
  const [error, setError] = useState<unknown>(),
    [isolate, setIsolate] = useState(false),
    [showLabels, setShowLabels] = useState(false),
    [hidden, setHidden] = useState<string[]>([]),
    [opacity, setOpacity] = useState(1),
    [clip, setClip] = useState(0),
    [loaded, setLoaded] = useState(false);
  const controlsState = useRef({ isolate, hidden, opacity, clip, showLabels });
  useEffect(() => {
    controlsState.current = { isolate, hidden, opacity, clip, showLabels };
  }, [isolate, hidden, opacity, clip, showLabels]);
  useEffect(() => {
    const container = host.current;
    if (!container || !file.data) return;
    let disposed = false,
      frame = 0,
      scene: THREE.Scene | undefined,
      renderer: THREE.WebGLRenderer | undefined,
      controls: OrbitControls | undefined,
      observer: ResizeObserver | undefined,
      cleanupListeners = () => {};
    const run = async () => {
      try {
        renderer = new THREE.WebGLRenderer({
          antialias: true,
          alpha: false,
          powerPreference: "low-power",
        });
        renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
        renderer.setClearColor("#edf1f5");
        renderer.localClippingEnabled = true;
        renderer.outputColorSpace = THREE.SRGBColorSpace;
        renderer.toneMapping = THREE.ACESFilmicToneMapping;
        renderer.shadowMap.enabled = true;
        renderer.shadowMap.type = THREE.PCFSoftShadowMap;
        container.append(renderer.domElement);
        const r = renderer;
        scene = new THREE.Scene();
        const s = scene;
        const camera = new THREE.PerspectiveCamera(40, 1, 0.01, 100000);
        controls = new OrbitControls(camera, r.domElement);
        const orbit = controls;
        orbit.enableDamping = false;
        orbit.screenSpacePanning = true;
        s.add(new THREE.HemisphereLight(0xddeeff, 0x635c4d, 1.8));
        const key = new THREE.DirectionalLight(0xfff1dd, 2.6);
        key.castShadow = true;
        key.shadow.mapSize.set(2048, 2048);
        key.position.set(-40, 70, 50);
        s.add(key);
        const fill = new THREE.DirectionalLight(0xb1ceff, 0.7);
        fill.position.set(50, 20, -30);
        s.add(fill);
        const gltf = await new GLTFLoader().parseAsync(
          await file.data!.arrayBuffer(),
          "",
        );
        if (disposed) {
          gltf.scene.traverse(disposeObject);
          return;
        }
        const model = gltf.scene;
        s.add(model);
        const box = new THREE.Box3().setFromObject(model),
          center = box.getCenter(new THREE.Vector3()),
          size = box.getSize(new THREE.Vector3()).length() || 1;
        camera.near = Math.max(size / 10000, 0.0001);
        camera.far = size * 1000;
        camera.aspect =
          container.clientWidth / Math.max(1, container.clientHeight);
        key.position
          .copy(center)
          .add(new THREE.Vector3(-0.5, 0.8, 0.6).multiplyScalar(size));
        key.target.position.copy(center);
        s.add(key.target);
        key.shadow.camera.left = key.shadow.camera.bottom = -size * 0.6;
        key.shadow.camera.right = key.shadow.camera.top = size * 0.6;
        key.shadow.camera.near = size * 0.1;
        key.shadow.camera.far = size * 3;
        key.shadow.normalBias = size * 0.0002;
        key.shadow.bias = -0.0001;
        key.shadow.camera.updateProjectionMatrix();
        camera.updateProjectionMatrix();
        const materialDefaults = new Map<
          THREE.Material,
          { opacity: number; transparent: boolean; color?: THREE.Color }
        >();
        model.traverse((o) => {
          if (o instanceof THREE.Mesh) {
            o.castShadow = true;
            o.receiveShadow = true;
            o.material = Array.isArray(o.material)
              ? o.material.map((m) => m.clone())
              : o.material.clone();
            for (const m of Array.isArray(o.material)
              ? o.material
              : [o.material]) {
              materialDefaults.set(m, {
                opacity: m.opacity,
                transparent: m.transparent,
                color:
                  "color" in m ? (m.color as THREE.Color).clone() : undefined,
              });
            }
          }
        });
        const instanceMatrices = new Map<
          THREE.InstancedMesh,
          THREE.Matrix4[]
        >();
        model.traverse((o) => {
          if (o instanceof THREE.InstancedMesh) {
            const list = [];
            for (let i = 0; i < o.count; i++) {
              const matrix = new THREE.Matrix4();
              o.getMatrixAt(i, matrix);
              list.push(matrix);
            }
            instanceMatrices.set(o, list);
          }
        });
        const labels: { node: HTMLDivElement; position: THREE.Vector3 }[] = [];
        const labelHost = document.createElement("div");
        labelHost.style.cssText =
          "position:absolute;inset:0;pointer-events:none;overflow:hidden";
        container.append(labelHost);
        const associations = gltf.parser.associations;
        const nodeIndex = (o: THREE.Object3D): number | undefined => {
          for (let n: THREE.Object3D | null = o; n; n = n.parent) {
            const index = associations.get(n)?.nodes;
            if (index !== undefined) return index;
          }
          return undefined;
        };
        const keys = (o: THREE.Object3D, instance?: number) => {
          const index = nodeIndex(o);
          const found = latest.current.elements
            .filter((e) =>
              e.fragments.some(
                (f) =>
                  f.node === index &&
                  (f.instance === undefined ||
                    f.instance === null ||
                    f.instance === instance),
              ),
            )
            .map((e) => e.key);
          if (found.length) return found;
          for (let node: THREE.Object3D | null = o; node; node = node.parent) {
            if (typeof node.userData.spatial_key === "string")
              return [
                node.userData.spatial_key +
                  (instance !== undefined ? `/instance:${instance}` : ""),
              ];
          }
          return [];
        };
        const markerGroup = new THREE.Group();
        s.add(markerGroup);
        const selectedBox = new THREE.Box3Helper(new THREE.Box3(), 0xc2410c);
        selectedBox.visible = false;
        s.add(selectedBox);
        const render = () => {
          if (disposed || frame) return;
          frame = requestAnimationFrame(() => {
            frame = 0;
            r.render(s, camera);
            for (const label of labels) {
              const p = label.position.clone().project(camera);
              label.node.hidden = p.z > 1 || p.z < -1;
              label.node.style.left = `${((p.x + 1) * container.clientWidth) / 2}px`;
              label.node.style.top = `${((1 - p.y) * container.clientHeight) / 2}px`;
            }
          });
        };
        const fit = (direction = "perspective") => {
          const p = latest.current.props;
          const point = p.placements.find((v) =>
            sameObject(p.selected, placementRef(v)),
          );
          const boxes: THREE.Box3[] = [];
          if (direction === "selection" && point?.anchor.length === 3) {
            const target = new THREE.Vector3(...point.anchor);
            boxes.push(
              new THREE.Box3().setFromCenterAndSize(
                target,
                new THREE.Vector3(12, 12, 12),
              ),
            );
          } else if (direction === "selection" && selectedBox.visible) {
            boxes.push(selectedBox.box.clone());
          } else {
            model.traverseVisible((o) => {
              if (!(o instanceof THREE.Mesh)) return;
              if (o instanceof THREE.InstancedMesh) {
                o.computeBoundingBox();
                if (o.boundingBox)
                  boxes.push(o.boundingBox.clone().applyMatrix4(o.matrixWorld));
              } else {
                o.geometry.computeBoundingBox();
                if (o.geometry.boundingBox)
                  boxes.push(
                    o.geometry.boundingBox.clone().applyMatrix4(o.matrixWorld),
                  );
              }
            });
          }
          const clipping = controlsState.current.clip;
          if (clipping > 0) {
            const cutX =
              box.min.x + (box.getSize(new THREE.Vector3()).x * clipping) / 100;
            for (const b of boxes) b.max.x = Math.min(b.max.x, cutX);
            for (let i = boxes.length - 1; i >= 0; i--)
              if (boxes[i]!.isEmpty()) boxes.splice(i, 1);
          }
          if (!boxes.length) return;
          const bounds = boxes.reduce(
            (all, b) => all.union(b),
            new THREE.Box3(),
          );
          const target = bounds.getCenter(new THREE.Vector3());
          const offset =
            direction === "top"
              ? new THREE.Vector3(0, 1, 0.001)
              : direction === "front"
                ? new THREE.Vector3(0, 0.001, 1)
                : direction === "side"
                  ? new THREE.Vector3(1, 0.001, 0)
                  : direction === "selection"
                    ? camera.position.clone().sub(orbit.target)
                    : new THREE.Vector3(-1, 0.7, 1);
          offset.normalize();
          const right = new THREE.Vector3()
            .crossVectors(camera.up, offset)
            .normalize();
          const up = new THREE.Vector3()
            .crossVectors(offset, right)
            .normalize();
          const tanV = Math.tan(THREE.MathUtils.degToRad(camera.fov / 2));
          const tanH = tanV * camera.aspect;
          let distance = camera.near * 10;
          for (const b of boxes)
            for (const x of [b.min.x, b.max.x])
              for (const y of [b.min.y, b.max.y])
                for (const z of [b.min.z, b.max.z]) {
                  const corner = new THREE.Vector3(x, y, z).sub(target);
                  distance = Math.max(
                    distance,
                    Math.abs(corner.dot(right)) / tanH + corner.dot(offset),
                    Math.abs(corner.dot(up)) / tanV + corner.dot(offset),
                  );
                }
          camera.position.copy(target).addScaledVector(offset, distance * 1.12);
          orbit.target.copy(target);
          orbit.update();
          render();
        };
        let focused = "";
        const apply = () => {
          const { props: p } = latest.current,
            state = controlsState.current;
          const selectedKeys = p.element
            ? [p.element]
            : p.links
                .filter((l) =>
                  sameObject(p.selected, {
                    kind: "component",
                    id: l.component_id,
                  }),
                )
                .map((l) => l.element_key);
          const selectedPlacement = p.placements.find((v) =>
            sameObject(p.selected, placementRef(v)),
          );
          if (selectedPlacement?.element_key)
            selectedKeys.push(selectedPlacement.element_key);
          const selectedBounds = new THREE.Box3();
          model.traverse((o) => {
            if (!(o instanceof THREE.Mesh)) return;
            let objectKeys = keys(o),
              match = objectKeys.some((k) => selectedKeys.includes(k));
            if (o instanceof THREE.InstancedMesh) {
              const matrices = instanceMatrices.get(o)!;
              let visibleCount = 0;
              for (let i = 0; i < o.count; i++) {
                const instanceKeys = keys(o, i),
                  selected = instanceKeys.some((k) => selectedKeys.includes(k)),
                  visible =
                    !instanceKeys.some(
                      (k) =>
                        state.hidden.includes(k) ||
                        p.settings.hidden_elements?.includes(k),
                    ) &&
                    (!state.isolate || !selectedKeys.length || selected);
                o.setMatrixAt(
                  i,
                  visible
                    ? matrices[i]!
                    : new THREE.Matrix4().makeScale(0, 0, 0),
                );
                o.setColorAt(
                  i,
                  new THREE.Color(selected ? 0xf59e0b : 0xffffff),
                );
                if (visible) visibleCount++;
                if (selected && visible) {
                  o.geometry.computeBoundingBox();
                  if (o.geometry.boundingBox)
                    selectedBounds.union(
                      o.geometry.boundingBox
                        .clone()
                        .applyMatrix4(
                          new THREE.Matrix4().multiplyMatrices(
                            o.matrixWorld,
                            matrices[i]!,
                          ),
                        ),
                    );
                }
              }
              o.instanceMatrix.needsUpdate = true;
              if (o.instanceColor) o.instanceColor.needsUpdate = true;
              o.visible = visibleCount > 0;
              objectKeys = [];
              match = false;
            }
            if (!(o instanceof THREE.InstancedMesh))
              o.visible =
                !objectKeys.some(
                  (k) =>
                    state.hidden.includes(k) ||
                    p.settings.hidden_elements?.includes(k),
                ) &&
                (!state.isolate || selectedKeys.length === 0 || match);
            if (match) selectedBounds.expandByObject(o);
            for (const material of Array.isArray(o.material)
              ? o.material
              : [o.material]) {
              const original = materialDefaults.get(material)!;
              material.opacity = original.opacity * state.opacity;
              material.transparent = original.transparent || state.opacity < 1;
              material.depthWrite = state.opacity >= 1;
              material.clippingPlanes = state.clip
                ? [
                    new THREE.Plane(
                      new THREE.Vector3(-1, 0, 0),
                      box.min.x +
                        (box.getSize(new THREE.Vector3()).x * state.clip) / 100,
                    ),
                  ]
                : [];
              if ("color" in material && original.color)
                (material.color as THREE.Color).copy(
                  match ? new THREE.Color(0xf59e0b) : original.color,
                );
              material.needsUpdate = true;
            }
          });
          labelHost.replaceChildren();
          labels.length = 0;
          if (state.clip > 0)
            selectedBounds.max.x = Math.min(
              selectedBounds.max.x,
              box.min.x +
                (box.getSize(new THREE.Vector3()).x * state.clip) / 100,
            );
          selectedBox.visible = !selectedBounds.isEmpty();
          if (selectedBox.visible) selectedBox.box.copy(selectedBounds);
          for (const child of [...markerGroup.children]) {
            markerGroup.remove(child);
            disposeObject(child);
          }
          for (const pnt of p.placements) {
            if (pnt.anchor.length !== 3) continue;
            if (
              state.isolate &&
              selectedKeys.length &&
              !selectedKeys.includes(pnt.element_key)
            )
              continue;
            if (
              state.clip > 0 &&
              pnt.anchor[0]! >
                box.min.x +
                  (box.getSize(new THREE.Vector3()).x * state.clip) / 100
            )
              continue;
            const selected = sameObject(p.selected, placementRef(pnt));
            const marker = new THREE.Mesh(
              new THREE.SphereGeometry(
                Math.min(
                  size * (selected ? 0.006 : 0.004),
                  selected ? 0.55 : 0.38,
                ),
                12,
                8,
              ),
              new THREE.MeshBasicMaterial({
                color: pnt.object?.retired
                  ? 0x64748b
                  : selected
                    ? 0xc2410c
                    : 0x0369a1,
                depthTest: false,
              }),
            );
            marker.position.fromArray(pnt.anchor);
            marker.renderOrder = 10;
            marker.userData.owner = placementRef(pnt);
            markerGroup.add(marker);
            const geometry=pnt.geometry;
            if(geometry) {
              let associated:THREE.Object3D|undefined;
              const color=selected?0xc2410c:0x0369a1;
              if(geometry.kind==='plane'&&geometry.normal&&geometry.size) {
                const plane=new THREE.Mesh(new THREE.PlaneGeometry(geometry.size[0]!,geometry.size[1]!),new THREE.MeshBasicMaterial({color,transparent:true,opacity:.25,side:THREE.DoubleSide,depthWrite:false}));
                plane.position.fromArray(pnt.anchor);plane.quaternion.setFromUnitVectors(new THREE.Vector3(0,0,1),new THREE.Vector3().fromArray(geometry.normal).normalize());associated=plane;
              } else if(geometry.vertices?.length) {
                const line=new THREE.BufferGeometry().setFromPoints(geometry.vertices.map(v=>new THREE.Vector3().fromArray(v)));const material=new THREE.LineBasicMaterial({color,depthTest:false});associated=geometry.kind==='region'?new THREE.LineLoop(line,material):new THREE.Line(line,material);
              }
              if(associated){associated.userData.owner=placementRef(pnt);associated.renderOrder=8;markerGroup.add(associated);}
            }
            const label = document.createElement("div");
            label.textContent = `${pnt.object?.retired ? "已退役 · " : ""}${pnt.object?.name ?? "测点"}${pnt.basis_changed ? " · 依据变化" : ""}`;
            label.style.cssText = `position:absolute;padding:3px 6px;background:rgba(255,255,255,.92);border-radius:3px;color:#0f172a;font:12px system-ui;transform:translate(${pnt.label_offset[0] ?? 12}px,${pnt.label_offset[1] ?? -12}px);white-space:nowrap`;
            if (state.showLabels || selected) {
              labelHost.append(label);
              labels.push({ node: label, position: marker.position.clone() });
            }
          }
          const focusKey = p.selected
            ? `${p.selected.kind}:${p.selected.id}`
            : (p.element ?? "");
          if (
            !p.editing &&
            focusKey &&
            focusKey !== focused &&
            (selectedPlacement || !selectedBounds.isEmpty())
          ) {
            const target = selectedPlacement
              ? new THREE.Vector3().fromArray(selectedPlacement.anchor)
              : selectedBounds.getCenter(new THREE.Vector3());
            camera.position.add(target.clone().sub(orbit.target));
            orbit.target.copy(target);
            orbit.update();
            focused = focusKey;
          }
          render();
        };
        const pick = (event: PointerEvent, hover: boolean) => {
          const rect = r.domElement.getBoundingClientRect(),
            pointer = new THREE.Vector2(
              ((event.clientX - rect.left) / rect.width) * 2 - 1,
              (-(event.clientY - rect.top) / rect.height) * 2 + 1,
            ),
            ray = new THREE.Raycaster();
          ray.setFromCamera(pointer, camera);
          const hit = ray
            .intersectObjects([...markerGroup.children, model], true)
            .find((h) => {
              for (
                let node: THREE.Object3D | null = h.object;
                node;
                node = node.parent
              )
                if (!node.visible) return false;
              if (
                controlsState.current.clip > 0 &&
                h.object instanceof THREE.Mesh
              ) {
                const materials = Array.isArray(h.object.material)
                  ? h.object.material
                  : [h.object.material];
                if (
                  materials.some((m) =>
                    m.clippingPlanes?.some(
                      (p: THREE.Plane) => p.distanceToPoint(h.point) < 0,
                    ),
                  )
                )
                  return false;
              }
              return true;
            });
          if (!hit) {
            if (hover) latest.current.props.onHover(undefined);
            return;
          }
          const element = keys(hit.object, hit.instanceId)[0],
            owner =
              hit.object.userData.owner ??
              latest.current.props.links.find((l) => l.element_key === element)
                ?.object;
          const ref = owner ? { kind: owner.kind, id: owner.id } : undefined;
          if (hover) latest.current.props.onHover(ref);
          else
            latest.current.props.onPick({
              owner: ref,
              element,
              anchor: hit.point.toArray(),
            });
        };
        let down = [0, 0];
        const start = (e: PointerEvent) => {
          down = [e.clientX, e.clientY];
        };
        const click = (e: PointerEvent) => {
          if (Math.hypot(e.clientX - down[0]!, e.clientY - down[1]!) < 5)
            pick(e, false);
        };
        const hover = (e: PointerEvent) => {
          if (e.buttons === 0) pick(e, true);
        };
        const leave = () => latest.current.props.onHover(undefined);
        r.domElement.addEventListener("pointerdown", start);
        r.domElement.addEventListener("pointerup", click);
        r.domElement.addEventListener("pointermove", hover);
        r.domElement.addEventListener("pointerleave", leave);
        cleanupListeners = () => {
          labelHost.remove();
          r.domElement.removeEventListener("pointerdown", start);
          r.domElement.removeEventListener("pointerup", click);
          r.domElement.removeEventListener("pointermove", hover);
          r.domElement.removeEventListener("pointerleave", leave);
        };
        orbit.addEventListener("change", render);
        observer = new ResizeObserver(() => {
          camera.aspect =
            container.clientWidth / Math.max(1, container.clientHeight);
          camera.updateProjectionMatrix();
          r.setSize(container.clientWidth, container.clientHeight);
          render();
        });
        observer.observe(container);
        api.current = {
          camera: (value) => {
            camera.position.fromArray(value.position);
            orbit.target.fromArray(value.target);
            orbit.update();
            render();
          },
          fit,
          apply,
          capture: () =>
            latest.current.props.onCamera?.({
              position: camera.position.toArray(),
              target: orbit.target.toArray(),
            }),
        };
        fit();
        const saved = latest.current.props.settings.camera;
        if (saved) {
          camera.position.fromArray(saved.position);
          orbit.target.fromArray(saved.target);
          orbit.update();
        }
        apply();
        setLoaded(true);
      } catch (e) {
        if (!disposed) setError(e);
      }
    };
    void run();
    return () => {
      disposed = true;
      cancelAnimationFrame(frame);
      observer?.disconnect();
      cleanupListeners();
      controls?.dispose();
      scene?.traverse(disposeObject);
      renderer?.dispose();
      renderer?.forceContextLoss();
      renderer?.domElement.remove();
      api.current = null;
    };
  }, [file.data]);
  useEffect(() => {
    if (props.settings.camera) api.current?.camera(props.settings.camera);
  }, [props.settings.camera]);
  useEffect(
    () => api.current?.apply(),
    [
      props.placements,
      props.links,
      props.selected,
      props.element,
      props.settings,
      isolate,
      showLabels,
      hidden,
      opacity,
      clip,
      elements,
    ],
  );
  return (
    <div className="relative h-full min-h-96">
      <div
        ref={host}
        role="region"
        aria-label="三维模型，拖动旋转、右键平移、滚轮缩放；构件目录提供键盘选择"
        className="relative h-full min-h-96"
      />
      <div className="absolute left-3 right-3 top-3 flex flex-wrap gap-2">
        {[
          ["perspective", "全景"],
          ["top", "俯视"],
          ["front", "正视"],
          ["side", "侧视"],
          ["selection", "聚焦所选"],
        ].map(([key, label]) => (
          <Button
            key={key}
            size="sm"
            variant="secondary"
            disabled={
              !loaded ||
              (key === "selection" &&
                !props.element &&
                !canPlace(props.selected?.kind) &&
                props.selected?.kind !== "component")
            }
            onClick={() => api.current?.fit(key)}
          >
            {label}
          </Button>
        ))}
        <Button
          size="sm"
          variant="secondary"
          disabled={!loaded || (!props.element && !props.selected)}
          aria-pressed={isolate}
          onClick={() => setIsolate(!isolate)}
        >
          隔离所选
        </Button>
        <Button
          size="sm"
          variant="secondary"
          disabled={!loaded || !props.element}
          onClick={() => {
            if (props.element) {
              const next = [
                ...new Set([
                  ...(props.settings.hidden_elements ?? []),
                  ...hidden,
                  props.element,
                ]),
              ];
              setHidden(next);
              props.onHiddenElements?.(next);
            }
          }}
        >
          隐藏所选
        </Button>
        <Button
          size="sm"
          variant="secondary"
          onClick={() => {
            setHidden([]);
            props.onHiddenElements?.([]);
            setIsolate(false);
            setOpacity(1);
            setClip(0);
          }}
        >
          重置可见性
        </Button>
        <Button
          size="sm"
          variant="secondary"
          aria-pressed={showLabels}
          onClick={() => setShowLabels(!showLabels)}
        >
          测点标签
        </Button>
        {props.onCamera && (
          <Button
            size="sm"
            variant="secondary"
            disabled={!loaded}
            onClick={() => api.current?.capture()}
          >
            记录当前视角
          </Button>
        )}
      </div>
      <div className="absolute bottom-3 left-3 right-3 flex flex-wrap gap-3 rounded bg-background/95 p-2 text-xs">
        <label className="flex items-center gap-2">
          透明度
          <input
            aria-label="模型透明度"
            type="range"
            min=".1"
            max="1"
            step=".1"
            value={opacity}
            onChange={(e) => setOpacity(Number(e.target.value))}
          />
        </label>
        <label className="flex items-center gap-2">
          X 向剖切
          <input
            aria-label="模型剖切"
            type="range"
            min="0"
            max="100"
            value={clip}
            onChange={(e) => setClip(Number(e.target.value))}
          />
        </label>
        <span>{loaded ? "视图坐标 · 米 · Y 向上" : "正在读取模型…"}</span>
      </div>
      {(error || file.error) && (
        <div className="absolute inset-x-3 bottom-16 bg-background">
          <Failure error={error ?? file.error} />
          <p className="p-3 text-sm">
            三维视图不可用，可继续通过构件目录查看属性和监测信息。
          </p>
        </div>
      )}
    </div>
  );
}
function disposeObject(o: THREE.Object3D) {
  if (o instanceof THREE.Mesh || o instanceof THREE.Line) {
    o.geometry?.dispose();
    for (const m of Array.isArray(o.material) ? o.material : [o.material]) {
      for (const value of Object.values(m))
        if (value instanceof THREE.Texture) value.dispose();
      m.dispose();
    }
  }
}
