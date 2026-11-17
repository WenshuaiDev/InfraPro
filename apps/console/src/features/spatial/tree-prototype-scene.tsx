// THROWAWAY schematic canvas. This is not a parser, a geographic basemap, or a BIM viewer.
import { useState } from 'react';
import { Crosshair, Minus, Plus, Scan, Compass } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { demoNodes, nodePath, type DemoAsset, type DemoBinding, type DemoNode } from './tree-prototype-data';

export function PrototypeScene({ asset, node, bindings, choose, placing = false, anchor, place }: { asset: DemoAsset; node: DemoNode; bindings: DemoBinding[]; choose?: (id: string) => void; placing?: boolean; anchor?: [number, number]; place?: (anchor: [number, number]) => void }) {
  const [zoom, setZoom] = useState(1);
  const [page, setPage] = useState('1');
  const markers = bindings.filter(binding => binding.asset === asset.id && demoNodes.find(item => item.id === binding.node)?.kind === 'point');
  const component = [...nodePath(node)].reverse().find(item => item.kind === 'component');
  const region = bindings.find(binding => binding.asset === asset.id && binding.node === component?.id);
  return <div className={`spt-scene ${placing ? 'is-placing' : ''}`}>
    <div className="spt-canvas-controls"><div className="spt-canvas-label"><span className="spt-dot" />{asset.kind === 'layout' ? '图纸预览' : asset.kind === 'model' ? '三维外观示意' : '地理位置示意'}<span className="spt-muted">· {asset.version}</span></div><div className="spt-controls">{asset.pages > 1 && !placing && <select aria-label="图纸页码" value={page} onChange={event => setPage(event.target.value)}><option value="1">第 1 页 · 平面图</option><option value="2">第 2 页 · 立面图</option></select>}<Button size="icon" variant="ghost" aria-label="缩小图纸" disabled={zoom <= 0.8} onClick={() => setZoom(Math.max(.8, zoom - .2))}><Minus /></Button><span>{Math.round(zoom * 100)}%</span><Button size="icon" variant="ghost" aria-label="放大图纸" disabled={zoom >= 1.6} onClick={() => setZoom(Math.min(1.6, zoom + .2))}><Plus /></Button><Button size="icon" variant="ghost" aria-label="适应画布" onClick={() => setZoom(1)}><Scan /></Button></div></div>
    <div className="spt-artboard">
      <div className="spt-drawing" style={{ transform: `scale(${zoom})` }}>
        <svg viewBox="0 0 900 480" preserveAspectRatio="none" role="img" aria-label={`${asset.name}，交互示意图`}>
          <defs><pattern id="spt-grid" width="24" height="24" patternUnits="userSpaceOnUse"><path d="M 24 0 L 0 0 0 24" fill="none" stroke="currentColor" strokeOpacity=".06" /></pattern></defs>
          <rect width="900" height="480" fill="url(#spt-grid)" />
          {asset.kind === 'geolayer' ? <>
            <path d="M520 -20 Q410 120 540 245 T500 510" fill="none" stroke="#b8d8de" strokeWidth="105" /><path d="M-20 290 Q260 210 460 270 T920 170" fill="none" stroke="#d3d7cf" strokeWidth="80" /><path d="M-20 290 Q260 210 460 270 T920 170" fill="none" stroke="#fff" strokeWidth="3" strokeDasharray="14 10" /><path d="M40 50 Q160 15 280 100 L210 170 L50 145Z M620 320 Q750 260 870 380 L850 460 L640 460Z" fill="#dbe5d9" /><text x="160" y="240" className="spt-svg-label">S218 沿江公路</text><text x="550" y="92" className="spt-svg-label">青澜江</text><circle cx="490" cy="250" r="12" fill="#14756b" stroke="white" strokeWidth="4" /><text x="515" y="245" className="spt-svg-heading">{demoNodes.find(item => item.id === asset.facility)?.name}</text>
          </> : asset.kind === 'model' ? <>
            <g transform="translate(75 75)"><path d="M60 230 L635 78 L760 139 L185 295Z" fill="#e2e9e8" stroke="#8b9c9e" strokeWidth="2" /><path d="M185 295 L760 139 L760 159 L185 315Z" fill="#bac8c7" stroke="#8b9c9e" /><path d="M60 230 L185 295 L185 315 L60 250Z" fill="#d1dcda" stroke="#8b9c9e" />{[0,1,2,3].map(i => <g key={i} transform={`translate(${125+i*157} ${246-i*42})`}><path d="M0 0 L26 -8 L26 79 L0 87Z" fill="#abbfbd" stroke="#8b9c9e" /><path d="M26 -8 L43 0 L43 86 L26 79Z" fill="#d6e1df" stroke="#8b9c9e" /></g>)}<path d="M60 230 L249 180 L311 210 L123 262Z" fill="#14756b" fillOpacity=".25" stroke="#14756b" strokeWidth="2" /><path d="M125 262 L700 109" stroke="white" strokeWidth="2" strokeDasharray="14 8" /></g><text x="80" y="425" className="spt-svg-label">选中区域：第1跨 · 左幅箱梁</text><text x="80" y="450" className="spt-svg-label">构件示意 · 透视视图</text>
          </> : asset.facility === 'tunnel' ? <>
            <path d="M220 360 V240 A230 200 0 0 1 460 65 A230 200 0 0 1 700 240 V360Z" fill="#e7ece9" stroke="#879b98" strokeWidth="3" /><path d="M250 350 V245 A200 173 0 0 1 460 96 A200 173 0 0 1 670 245 V350Z" fill="var(--spt-paper)" stroke="#879b98" strokeWidth="2" /><path d="M200 370 H720 M460 55 V390" stroke="#9ba9a7" strokeDasharray="7 6" /><text x="340" y="425" className="spt-svg-heading">进口第1监测断面</text><circle cx="460" cy="96" r="7" fill="#14756b" />
          </> : asset.facility === 'slope' ? <>
            <path d="M100 380 L290 380 L340 290 L430 290 L480 200 L570 200 L630 100 L800 100 V420 H100Z" fill="#e1e8dc" stroke="#879b81" strokeWidth="2" />{[0,1,2].map(i => <path key={i} d={`M${300+i*145} ${350-i*90} l80 40`} stroke="#92a3a0" strokeWidth="4" />)}<text x="100" y="130" className="spt-svg-heading">南岭高边坡 · 监测剖面</text><text x="600" y="85" className="spt-svg-label">坡顶</text>
          </> : page === '2' ? <>
            <path d="M105 210 H795 V245 H105Z" fill="#e6ecea" stroke="#91a6a2" strokeWidth="2" />{[150,350,550,750].map(x => <g key={x}><rect x={x} y="245" width="20" height="110" fill="#d5dfdb" stroke="#91a6a2" /><path d={`M${x-20} 355 h60`} stroke="#91a6a2" strokeWidth="5" /></g>)}<path d="M70 370 Q280 350 460 380 T830 360" fill="none" stroke="#b0cdd1" strokeWidth="2" /><text x="350" y="130" className="spt-svg-heading">青澜大桥 · 立面图</text>
          </> : <>
            <path d="M410 0 Q370 130 460 260 T450 480" fill="none" stroke="#dcebed" strokeWidth="190" /><text x="452" y="82" className="spt-svg-label">青 澜 江</text>
            <g stroke="#9caeab" fill="none" strokeWidth="1.4"><path d="M96 168 H804 V225 H96Z M96 263 H804 V320 H96Z" fill="var(--spt-paper)" /><path d="M96 157 H804 M96 236 H804 M96 252 H804 M96 331 H804" /><path d="M96 197 H804 M96 292 H804" strokeDasharray="12 8" /><path d="M153 146 V341 M350 146 V341 M550 146 V341 M747 146 V341" strokeDasharray="5 5" />{[153,350,550,747].map(x => <g key={x}><rect x={x-7} y="145" width="14" height="198" fill="#e1e8e4" /><path d={`M${x} 123 v-18 M${x} 105 h${x===747?0:197}`} /></g>)}</g>
            {region && <rect x={region.anchor[0]*9-90} y={region.anchor[1]*4.8-24} width="179" height="49" rx="3" fill="#14756b" fillOpacity=".13" stroke="#14756b" strokeWidth="2" />}
            <text x="219" y="132" className="spt-svg-label">第1跨 · 40m</text><text x="416" y="132" className="spt-svg-label">第2跨 · 40m</text><text x="613" y="132" className="spt-svg-label">第3跨 · 40m</text>
            <text x="51" y="201" className="spt-svg-label">左幅</text><text x="51" y="296" className="spt-svg-label">右幅</text><path d="M160 365 H745 M160 359 V371 M745 359 V371" stroke="#8ca09c" /><text x="411" y="388" className="spt-svg-label">桥梁总长 120 m</text>
            <text x="96" y="438" className="spt-svg-heading">{asset.name}</text><text x="96" y="460" className="spt-svg-label">监测平面布置 · 示意图</text>
          </>}
        </svg>
        {placing && <button type="button" className="spt-placement-surface" aria-label="在图中放置当前对象，键盘回车使用画布中心" onClick={event => { const rect = event.currentTarget.getBoundingClientRect(); place?.(event.detail === 0 ? [50, 50] : [Math.round((event.clientX-rect.left)/rect.width*100), Math.round((event.clientY-rect.top)/rect.height*100)]); }} />}
        {page === '1' && markers.map(binding => { const point = demoNodes.find(item => item.id === binding.node)!; return <button key={binding.node} type="button" className={`spt-map-point ${node.id === point.id ? 'is-active' : ''}`} style={{ left: `${binding.anchor[0]}%`, top: `${binding.anchor[1]}%` }} aria-label={`图中选择 ${point.name} ${point.code}`} onClick={() => choose?.(point.id)} disabled={placing}><span /><em>{point.code}</em></button>; })}
        {anchor && <span className="spt-pending-anchor" style={{ left: `${anchor[0]}%`, top: `${anchor[1]}%` }}><Crosshair size={26} /><em>{node.name}</em></span>}
      </div>
      <span className="spt-north"><Compass size={24} /><small>N</small></span>
    </div>
    <div className="spt-scene-legend"><span><i className="spt-dot" />已定位测点</span><span><i className="spt-region-dot" />当前构件区域</span><span className="spt-muted">{placing ? '点击画布指定位置；支持键盘回车选择中心' : '点击测点，定位到左侧目录'}</span></div>
  </div>;
}
