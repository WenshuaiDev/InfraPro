import { useEffect, useMemo, useRef, useState } from 'react';
import zipScript from 'jszip/dist/jszip.min.js?raw';
import docxScript from '../../../../node_modules/docx-preview/dist/docx-preview.min.js?raw';
import bridgeScript from './docx-bridge.js?raw';
import { Button } from '@infrapro/ui/components/button';
import { PageControls, PreviewFrame, SearchControl, ZoomControls } from './controls';

export default function DocxPreview({ blob, title, onRetry }: { blob: Blob; title: string; onRetry: () => void }) {
  const frame = useRef<HTMLIFrameElement>(null), [channel] = useState(() => crypto.randomUUID());
  const [page, setPage] = useState(1), [count, setCount] = useState(1), [zoom, setZoom] = useState(1), [query, setQuery] = useState('');
  const [matches, setMatches] = useState(''), [ready, setReady] = useState(false), [error, setError] = useState('');
  const html = useMemo(() => {
    const script = (source: string) => `<script nonce="${channel}">${source.replace(/<\/script/gi, '<\\/script')}</script>`;
    return `<!doctype html><html lang="zh-CN" data-channel="${channel}"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'nonce-${channel}'; style-src 'unsafe-inline'; img-src data: blob:; font-src data: blob:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"><style>html{color-scheme:light}body{margin:0;background:#e5e7eb;color:#111827;font-family:Arial,'PingFang SC',sans-serif}#document{width:max-content;min-width:100%;transform-origin:top left}.docx-wrapper{padding:12px!important;background:transparent!important;align-items:flex-start!important}.docx-wrapper>section.docx{margin:0 auto 12px!important}mark{color:#111827}a{color:inherit;pointer-events:none}section{content-visibility:auto;contain-intrinsic-size:auto 1100px}</style><div id="styles"></div></head><body><main id="document"></main>${script(zipScript)}${script(docxScript)}${script(bridgeScript)}</body></html>`;
  }, [channel]);
  const send = (command: object) => frame.current?.contentWindow?.postMessage({ channel, ...command }, '*');
  useEffect(() => {
    let stopped = false;
    const timeout = setTimeout(() => { setError('Word 解析超过 30 秒，请下载原件查看。'); }, 30000);
    const receive = (event: MessageEvent) => {
      if (stopped || event.source !== frame.current?.contentWindow || event.data?.channel !== channel) return;
      const message = event.data;
      if (message.kind === 'ready') { clearTimeout(timeout); setReady(true); setCount(message.count); }
      if (message.kind === 'page') setPage(message.page);
      if (message.kind === 'zoom') setZoom(message.zoom);
      if (message.kind === 'matches') setMatches(`${message.current} / ${message.total}${message.total === 2000 ? '+' : ''} 处`);
      if (message.kind === 'error') { clearTimeout(timeout); setError(message.error); }
    };
    window.addEventListener('message', receive);
    return () => { stopped = true; clearTimeout(timeout); window.removeEventListener('message', receive); };
  }, [channel]);
  useEffect(() => { if (!ready) return; const timer = setTimeout(() => frame.current?.contentWindow?.postMessage({ channel, kind: 'search', query }, '*'), 200); return () => clearTimeout(timer); }, [query, ready, channel]);
  return <PreviewFrame title={title} tools={<><PageControls page={page} count={count} onPage={value => send({ kind: 'page', page: value })} /><ZoomControls zoom={zoom} onZoom={value => send({ kind: 'zoom', zoom: value, fit: false })} onFit={() => send({ kind: 'zoom', zoom: 1, fit: true })} /><SearchControl value={query} onChange={setQuery} count={matches} onNext={() => send({ kind: 'match' })} onPrevious={() => send({ kind: 'match', previous: true })} /></>} status={error || (ready ? `${count} 个文档分节页 · 复杂版式与 Word 可能略有差异；以原件为准。` : '正在排版 Word 文档…')}>
    {error ? <div className="space-y-3 p-6 text-sm"><p role="alert">{error}</p><Button variant="outline" onClick={onRetry}>重试预览</Button></div> : <iframe ref={frame} title={`${title} Word 阅读区`} sandbox="allow-scripts" referrerPolicy="no-referrer" srcDoc={html} className="h-full w-full border-0" onLoad={() => { setReady(false); const target = frame.current?.contentWindow; void blob.arrayBuffer().then(bytes => { if (target && target === frame.current?.contentWindow) target.postMessage({ channel, kind: 'load', bytes }, '*', [bytes]); }).catch(() => setError('Word 正文读取失败，请重试或下载原件。')); }} />}
  </PreviewFrame>;
}
