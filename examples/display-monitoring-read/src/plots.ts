import type {ReadMapFeature,ReadTrendSeries} from '@infrapro/display-sdk';
const ns='http://www.w3.org/2000/svg';
function svg(label:string){const s=document.createElementNS(ns,'svg');s.setAttribute('viewBox','0 0 720 330');s.setAttribute('class','map');s.setAttribute('role','img');s.setAttribute('aria-label',label);return s;}
function path(host:SVGElement,d:string,closed=false){const p=document.createElementNS(ns,'path');p.setAttribute('d',d);p.setAttribute('stroke','#65e49b');p.setAttribute('stroke-width','2');p.setAttribute('fill',closed?'#65e49b33':'none');p.setAttribute('fill-rule','evenodd');host.append(p);}
export function drawFeatures(host: HTMLElement, features: ReadMapFeature[]) {
  const output = svg('当前页获准 GIS 要素，适配经纬范围，非测绘底图');
  type Pair = [number, number];
  const coordinate = (v: unknown): Pair | null => {
    if (!Array.isArray(v) || typeof v[0] !== 'number' || typeof v[1] !== 'number' || !Number.isFinite(v[0]) || !Number.isFinite(v[1])) return null;
    return [v[0], v[1]];
  };
  const positions: Pair[] = [];
  const collect = (v: unknown): void => {
    const pair = coordinate(v);
    if (pair) positions.push(pair);
    else if (Array.isArray(v)) for (const item of v) collect(item);
  };
  for (const feature of features) collect(feature.geometry.coordinates);
  if (!positions.length) { host.append(output); return; }
  let west = Infinity, east = -Infinity, south = Infinity, north = -Infinity;
  for (const [x, y] of positions) { west = Math.min(west, x); east = Math.max(east, x); south = Math.min(south, y); north = Math.max(north, y); }
  // Fit the current authorized page so local linework remains visible. This
  // is a coordinate overview, not a source of survey distances or areas.
  const scale = Math.min(680 / Math.max(east - west, .000001), 290 / Math.max(north - south, .000001));
  const point = (v: unknown): Pair | null => {
    const p = coordinate(v);
    return p ? [360 + (p[0] - (west + east) / 2) * scale, 165 - (p[1] - (south + north) / 2) * scale] : null;
  };
  const line = (values: unknown): string => {
    if (!Array.isArray(values)) return '';
    return values.map(point).filter((v): v is Pair => v !== null).map(([x, y], i) => `${i ? 'L' : 'M'}${x},${y}`).join(' ');
  };
  const marker = (value: unknown) => {
    const p = point(value); if (!p) return;
    const circle = document.createElementNS(ns, 'circle');
    circle.setAttribute('cx', String(p[0])); circle.setAttribute('cy', String(p[1])); circle.setAttribute('r', '5'); output.append(circle);
  };
  const polygon = (rings: unknown) => {
    if (!Array.isArray(rings)) return;
    const d = rings.map(line).filter(Boolean).map(ring => ring + ' Z').join(' ');
    if (d) path(output, d, true);
  };
  for (const feature of features) {
    const type = feature.geometry.type, coordinates = feature.geometry.coordinates;
    if (type === 'Point') marker(coordinates);
    else if (type === 'MultiPoint' && Array.isArray(coordinates)) for (const p of coordinates) marker(p);
    else if (type === 'LineString') path(output, line(coordinates));
    else if (type === 'MultiLineString' && Array.isArray(coordinates)) for (const values of coordinates) path(output, line(values));
    else if (type === 'Polygon') polygon(coordinates);
    else if (type === 'MultiPolygon' && Array.isArray(coordinates)) for (const rings of coordinates) polygon(rings);
  }
  host.append(output);
}
/** Exact decimal differences determine plot positions; the original value remains the label. */
export function drawTrends(host:HTMLElement,series:ReadTrendSeries[]){for(const row of series){
 const values=row.buckets.flatMap(b=>b.points).filter(p=>p.value!==null);
 if(!values.length)continue;
 const parsed=values.map(p=>/^(-?)(\d+)(?:\.(\d+))?$/.exec(p.value!));if(parsed.some(v=>!v))continue;
 const scale=Math.max(...parsed.map(v=>v![3]?.length??0));
 const integers=parsed.map(v=>BigInt(`${v![1]}${v![2]}${(v![3]??'').padEnd(scale,'0')}`));
 const min=integers.reduce((a,b)=>a<b?a:b),max=integers.reduce((a,b)=>a>b?a:b),range=max-min||1n,output=svg(`通道 ${row.channel_id} 趋势`);
 const heading=document.createElement('h3');heading.textContent=`通道 ${row.channel_id}`;host.append(heading);
 // Bucket gaps remain separate paths; they are never joined into a continuous record.
 let index=0;
 for(const bucket of row.buckets){let d='';for(const item of bucket.points){if(item.value===null){if(d)path(output,d);d='';continue;}const x=20+index*680/Math.max(values.length-1,1),y=300-Number((integers[index]-min)*270n/range);d+=`${d?' L':'M'}${x},${y}`;const c=document.createElementNS(ns,'circle');c.setAttribute('cx',String(x));c.setAttribute('cy',String(y));c.setAttribute('r','3');const title=document.createElementNS(ns,'title');title.textContent=item.value;c.append(title);output.append(c);index++;}if(d)path(output,d);}
 const title=document.createElementNS(ns,'text');title.setAttribute('x','20');title.setAttribute('y','20');title.textContent=`${values[0].value} → ${values.at(-1)!.value} · ${row.coverage}`;output.append(title);host.append(output);
}}
