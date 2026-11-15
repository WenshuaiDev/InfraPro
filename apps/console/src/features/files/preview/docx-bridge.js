/* Runs only inside the opaque-origin document sandbox, never in the console. */
(() => {
  const channel = document.documentElement.dataset.channel;
  const root = document.getElementById('document');
  let pages = [], matches = [], current = -1, zoom = 1, fit = true, loaded = false;
  const send = data => parent.postMessage({ channel, ...data }, '*');
  const scale = () => {
    const width = pages[0]?.offsetWidth || 794;
    if (fit) zoom = Math.max(0.1, Math.min(2, (innerWidth - 24) / width));
    root.style.zoom = String(zoom);
    send({ kind: 'zoom', zoom });
  };
  const pageAt = () => {
    let page = 1;
    pages.forEach((element, index) => { if (element.getBoundingClientRect().top < innerHeight / 2) page = index + 1; });
    send({ kind: 'page', page });
  };
  let scrollScheduled = false;
  addEventListener('scroll', () => { if (!scrollScheduled) { scrollScheduled = true; requestAnimationFrame(() => { scrollScheduled = false; pageAt(); }); } });
  addEventListener('resize', scale);
  document.addEventListener('click', event => { if (event.target.closest?.('a')) event.preventDefault(); }, true);
  document.addEventListener('auxclick', event => event.preventDefault(), true);
  const locate = index => {
    if (!matches.length) { send({ kind: 'matches', current: 0, total: 0 }); return; }
    current = (index + matches.length) % matches.length;
    matches.forEach((mark, i) => { mark.style.background = i === current ? '#fb923c' : '#fef08a'; });
    matches[current].scrollIntoView({ block: 'center' });
    send({ kind: 'matches', current: current + 1, total: matches.length });
  };
  const search = query => {
    for (const mark of matches) mark.replaceWith(document.createTextNode(mark.textContent));
    root.normalize(); matches = []; current = -1;
    if (!query) { send({ kind: 'matches', current: 0, total: 0 }); return; }
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT), nodes = [];
    while (walker.nextNode()) if (!walker.currentNode.parentElement.closest('style,script')) nodes.push(walker.currentNode);
    for (const node of nodes) {
      const text = node.textContent, lower = text.toLocaleLowerCase(), needle = query.toLocaleLowerCase();
      let cursor = 0, at = lower.indexOf(needle), changed = false;
      const fragment = document.createDocumentFragment();
      while (at >= 0 && matches.length < 2000) {
        fragment.append(document.createTextNode(text.slice(cursor, at)));
        const mark = document.createElement('mark'); mark.textContent = text.slice(at, at + query.length); fragment.append(mark); matches.push(mark);
        cursor = at + query.length; at = lower.indexOf(needle, cursor); changed = true;
      }
      if (changed) { fragment.append(document.createTextNode(text.slice(cursor))); node.replaceWith(fragment); }
      if (matches.length >= 2000) break;
    }
    locate(0);
  };
  addEventListener('message', async event => {
    if (event.source !== parent || event.data?.channel !== channel) return;
    const command = event.data;
    try {
      if (command.kind === 'load' && !loaded) {
        loaded = true;
        await window.docx.renderAsync(command.bytes, root, document.getElementById('styles'), { className: 'docx', inWrapper: true, breakPages: true, ignoreLastRenderedPageBreak: false, renderHeaders: true, renderFooters: true, renderFootnotes: true, renderEndnotes: true, renderComments: false, renderChanges: false, renderAltChunks: false, useBase64URL: true, experimental: false });
        root.querySelectorAll('a').forEach(link => { link.removeAttribute('href'); link.removeAttribute('target'); });
        pages = [...root.querySelectorAll('section.docx')];
        if (!pages.length || pages.length > 2000) throw Error('文档分页为空或超过 2000 页。');
        scale(); send({ kind: 'ready', count: pages.length }); pageAt();
      } else if (command.kind === 'page') pages[Math.max(0, Math.min(pages.length - 1, command.page - 1))]?.scrollIntoView({ block: 'start' });
      else if (command.kind === 'zoom') { fit = command.fit; zoom = Math.max(0.1, Math.min(3, command.zoom)); scale(); }
      else if (command.kind === 'search') search(String(command.query).slice(0, 500));
      else if (command.kind === 'match') locate(current + (command.previous ? -1 : 1));
    } catch { send({ kind: 'error', error: 'Word 文档无法预览，文件可能损坏、加密或版式不受支持。请下载原件查看。' }); }
  });
})();
