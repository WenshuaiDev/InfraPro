export const positionLabels: Record<string, string> = { description: '位置说明', route_code: '路线编号', route_name: '路线名称', reference_name: '线位、方向或桩号依据', start_station: '起点或点位桩号', end_station: '终点桩号', side: '侧别', direction: '方向', offset_m: '偏距（m）', note: '备注', crs: '坐标系依据', longitude: '经度（度）', latitude: '纬度（度）', x: 'X', y: 'Y', z: 'Z', axis_description: '坐标轴说明', unit: '长度单位', value_m: '高程（m）', datum: '高程基准', reference_description: '参照说明', axes_description: '局部坐标轴说明', code: '断面编号', name: '断面名称', source_reference: '位置资料来源', source_date: '资料日期（YYYY-MM-DD）', accuracy_m: '已知精度（m）' };
const groups: Record<string,string> = {geographic:'地理坐标',projected:'投影坐标',elevation:'高程',local:'工程局部位置',linear_references:'路线参照',sections:'断面参照'};
export function positionIssueLabel(path:string) {
 const parts=path.split('/').filter(Boolean).slice(1);
 const group=groups[parts[0]??''];const key=parts.at(-1)??'';const index=parts[1];
 return [group,index&&/^\d+$/.test(index)?`第 ${Number(index)+1} 项`:undefined,positionLabels[key]??(key==='kind'?'断面种类':undefined)].filter(Boolean).join(' · ')||'位置资料';
}
