// THROWAWAY: illustrative data for the facility-first spatial UI, never sent to the API.
export type PrototypeVariant = 'A' | 'B' | 'C';
export type DemoNode = { id: string; name: string; code: string; kind: 'facility' | 'component' | 'point'; parent?: string; facility: string };
export type DemoAsset = { id: string; facility: string; name: string; kind: 'layout' | 'model' | 'geolayer'; version: string; pages: number };
export type DemoBinding = { node: string; asset: string; anchor: [number, number]; detail: string };
export const demoNodes: DemoNode[] = [
  { id: 'bridge', name: '青澜大桥', code: 'QL-BR-001', kind: 'facility', facility: 'bridge' },
  { id: 'span1', name: '第1跨主梁', code: 'QL-L01', kind: 'component', parent: 'bridge', facility: 'bridge' },
  { id: 'beam1', name: '左幅箱梁', code: 'QL-L01-L', kind: 'component', parent: 'span1', facility: 'bridge' },
  { id: 'p1', name: '跨中挠度测点', code: 'ND-01', kind: 'point', parent: 'beam1', facility: 'bridge' },
  { id: 'p2', name: '梁底应变测点', code: 'YB-01', kind: 'point', parent: 'beam1', facility: 'bridge' },
  { id: 'beam2', name: '右幅箱梁', code: 'QL-L01-R', kind: 'component', parent: 'span1', facility: 'bridge' },
  { id: 'p3', name: '跨中挠度测点', code: 'ND-02', kind: 'point', parent: 'beam2', facility: 'bridge' },
  { id: 'span2', name: '第2跨主梁', code: 'QL-L02', kind: 'component', parent: 'bridge', facility: 'bridge' },
  { id: 'p4', name: '跨中位移测点', code: 'WY-02', kind: 'point', parent: 'span2', facility: 'bridge' },
  { id: 'span3', name: '第3跨主梁', code: 'QL-L03', kind: 'component', parent: 'bridge', facility: 'bridge' },
  { id: 'p5', name: '桥面温度测点', code: 'WD-01', kind: 'point', parent: 'bridge', facility: 'bridge' },
  { id: 'tunnel', name: '北山隧道', code: 'BS-TN-001', kind: 'facility', facility: 'tunnel' },
  { id: 'section1', name: '进口第1监测断面', code: 'BS-D01', kind: 'component', parent: 'tunnel', facility: 'tunnel' },
  { id: 'tp1', name: '拱顶沉降测点', code: 'CJ-01', kind: 'point', parent: 'section1', facility: 'tunnel' },
  { id: 'slope', name: '南岭高边坡', code: 'NL-SL-001', kind: 'facility', facility: 'slope' },
  { id: 'slope1', name: '第1级坡面', code: 'NL-P01', kind: 'component', parent: 'slope', facility: 'slope' },
  { id: 'sp1', name: '坡顶位移测点', code: 'WY-01', kind: 'point', parent: 'slope1', facility: 'slope' },
  { id: 'road', name: '沿江公路东段', code: 'S218-E', kind: 'facility', facility: 'road' },
  { id: 'road1', name: '沥青面层', code: 'S218-M01', kind: 'component', parent: 'road', facility: 'road' },
];
export const demoAssets: DemoAsset[] = [
  { id: 'bridge-plan', facility: 'bridge', name: '青澜大桥监测布设总图', kind: 'layout', version: 'V3', pages: 2 },
  { id: 'bridge-detail', facility: 'bridge', name: '第1跨主梁构造详图', kind: 'layout', version: 'V2', pages: 1 },
  { id: 'bridge-model', facility: 'bridge', name: '青澜大桥结构模型', kind: 'model', version: 'V2', pages: 1 },
  { id: 'bridge-gis', facility: 'bridge', name: '桥位及周边地理要素', kind: 'geolayer', version: 'V1', pages: 1 },
  { id: 'tunnel-plan', facility: 'tunnel', name: '北山隧道断面布设图', kind: 'layout', version: 'V1', pages: 1 },
  { id: 'slope-plan', facility: 'slope', name: '南岭边坡监测布设图', kind: 'layout', version: 'V1', pages: 1 },
];
export const demoBindings: DemoBinding[] = [
  { node: 'bridge', asset: 'bridge-plan', anchor: [50, 50], detail: '整份资料' },
  { node: 'bridge', asset: 'bridge-model', anchor: [50, 50], detail: '整体模型' },
  { node: 'bridge', asset: 'bridge-gis', anchor: [50, 50], detail: '设施位置' },
  { node: 'span1', asset: 'bridge-plan', anchor: [28, 42], detail: '第 1 页 · 第1跨区域' },
  { node: 'span1', asset: 'bridge-detail', anchor: [28, 42], detail: '整份资料' },
  { node: 'beam1', asset: 'bridge-plan', anchor: [28, 42], detail: '第 1 页 · 左幅箱梁区域' },
  { node: 'beam1', asset: 'bridge-model', anchor: [28, 42], detail: '模型构件 · BOX-GIRDER-L01' },
  { node: 'p1', asset: 'bridge-plan', anchor: [28, 41], detail: '第 1 页 · 平面坐标' },
  { node: 'beam2', asset: 'bridge-plan', anchor: [28, 61], detail: '第 1 页 · 右幅箱梁区域' },
  { node: 'p3', asset: 'bridge-plan', anchor: [28, 61], detail: '第 1 页 · 平面坐标' },
  { node: 'span2', asset: 'bridge-plan', anchor: [50, 42], detail: '第 1 页 · 第2跨区域' },
  { node: 'tunnel', asset: 'tunnel-plan', anchor: [50, 50], detail: '整份资料' },
];
export const nodeKindLabels = { facility: '设施', component: '构件', point: '测点' };
export const assetKindLabels = { layout: '布设图', model: 'BIM / 三维', geolayer: 'GIS 图层' };
export function nodePath(node: DemoNode): DemoNode[] {
  const parent = demoNodes.find(item => item.id === node.parent);
  return [...(parent ? nodePath(parent) : []), node];
}
export function nodeStatus(node: DemoNode, bindings: DemoBinding[]) {
  return bindings.some(binding => binding.node === node.id) ? '已绑定' : nodePath(node).slice(0, -1).some(parent => bindings.some(binding => binding.node === parent.id)) ? '待定位' : '未绑定';
}
