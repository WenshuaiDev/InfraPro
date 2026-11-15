import { useEffect, useRef, useState } from 'react';
import { AnnotationMode, GlobalWorkerOptions, getDocument, type PDFDocumentLoadingTask } from 'pdfjs-dist';
import { EventBus, PDFFindController, PDFLinkService, PDFViewer } from 'pdfjs-dist/web/pdf_viewer.mjs';
import workerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url';
import 'pdfjs-dist/web/pdf_viewer.css';
import { Button } from '@infrapro/ui/components/button';
import { PageControls, PreviewFrame, SearchControl, ZoomControls } from './controls';

GlobalWorkerOptions.workerSrc = workerURL;

export default function PdfPreview({ blob, title, onRetry }: { blob: Blob; title: string; onRetry: () => void }) {
  const container = useRef<HTMLDivElement>(null), viewer = useRef<PDFViewer | null>(null), bus = useRef<EventBus | null>(null);
  const [page, setPage] = useState(1), [count, setCount] = useState(1), [zoom, setZoom] = useState(1), [query, setQuery] = useState('');
  const [matches, setMatches] = useState(''), [error, setError] = useState(''), [ready, setReady] = useState(false);
  useEffect(() => {
    const element = container.current;
    if (!element) return;
    let stopped = false, task: PDFDocumentLoadingTask | undefined;
    const events = new EventBus(), links = new PDFLinkService({ eventBus: events }), find = new PDFFindController({ eventBus: events, linkService: links });
    links.externalLinkEnabled = false;
    const pdf = new PDFViewer({ container: element, eventBus: events, linkService: links, findController: find, annotationMode: AnnotationMode.DISABLE, maxCanvasPixels: 8 * 1024 ** 2 });
    viewer.current = pdf; bus.current = events; links.setViewer(pdf);
    events.on('pagesinit', () => { if (!stopped) { pdf.currentScaleValue = 'page-width'; setReady(true); } });
    events.on('pagechanging', ({ pageNumber }: { pageNumber: number }) => { if (!stopped) setPage(pageNumber); });
    events.on('scalechanging', ({ scale }: { scale: number }) => { if (!stopped) setZoom(scale); });
    const updateMatches = ({ matchesCount }: { matchesCount: { current: number; total: number } }) => { if (!stopped && matchesCount) setMatches(`${matchesCount.current} / ${matchesCount.total} 处`); };
    events.on('updatefindmatchescount', updateMatches); events.on('updatefindcontrolstate', updateMatches);
    const resize = new ResizeObserver(() => { if (pdf.currentScaleValue === 'page-width') pdf.currentScaleValue = 'page-width'; pdf.update(); }); resize.observe(element);
    const timeout = setTimeout(() => { if (!stopped) { setError('PDF 加载超时，请重试或下载原件。'); void task?.destroy(); } }, 60000);
    void (async () => {
      const bytes = await blob.arrayBuffer(); if (stopped) return;
      const assets = new URL(`${import.meta.env.BASE_URL}vendor/pdfjs/`, window.location.origin).href;
      task = getDocument({ data: bytes, cMapUrl: assets + 'cmaps/', cMapPacked: true, standardFontDataUrl: assets + 'standard_fonts/', wasmUrl: assets + 'wasm/', iccUrl: assets + 'iccs/', enableXfa: false });
      task.onPassword = () => { if (!stopped) setError('此 PDF 已加密，请下载原件并使用密码打开。'); void task?.destroy(); };
      const document = await task.promise;
      if (stopped) return;
      if (document.numPages > 2000) throw Error('PDF 超过 2000 页，请下载原件查看。');
      clearTimeout(timeout); setCount(document.numPages); links.setDocument(document); pdf.setDocument(document);
    })().catch(cause => { if (!stopped) setError(previous => previous || (cause instanceof Error ? `PDF 无法预览：${cause.message}` : 'PDF 无法预览，请下载原件。')); });
    return () => { stopped = true; clearTimeout(timeout); resize.disconnect(); bus.current = null; viewer.current = null; pdf.setDocument(null); links.setDocument(null); void task?.destroy(); };
  }, [blob]);
  const search = (value: string, again = false, previous = false) => { setQuery(value); bus.current?.dispatch('find', { type: again ? 'again' : '', query: value, caseSensitive: false, entireWord: false, highlightAll: true, findPrevious: previous, matchDiacritics: false }); };
  return <PreviewFrame title={title} tools={<><PageControls page={page} count={count} onPage={value => { if (ready && viewer.current) viewer.current.currentPageNumber = value; }} /><ZoomControls zoom={zoom} onZoom={value => { if (ready && viewer.current) viewer.current.currentScale = value; }} onFit={() => { if (ready && viewer.current) viewer.current.currentScaleValue = 'page-width'; }} onRotate={() => { if (ready && viewer.current) viewer.current.pagesRotation = (viewer.current.pagesRotation + 90) % 360; }} /><SearchControl value={query} onChange={value => search(value)} onNext={() => search(query, true)} onPrevious={() => search(query, true, true)} count={matches} /></>} status={error || (ready ? `${count} 页 · 可连续滚动、选择文字；扫描件需下载后另行识别文字。` : '正在载入 PDF…')}>
    <div ref={container} data-pdf-container className="absolute inset-0 overflow-auto bg-muted/40" tabIndex={0} aria-label="PDF 页面"><div className="pdfViewer" /></div>
    {error && <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-background p-6 text-sm"><p role="alert">{error}</p><Button variant="outline" onClick={onRetry}>重试预览</Button></div>}
  </PreviewFrame>;
}
