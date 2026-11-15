import { parseDocument } from './parse';
import type { ParseRequest } from './model';
self.onmessage = (event: MessageEvent<ParseRequest>) => {
  try { self.postMessage({ result: parseDocument(event.data) }); }
  catch (error) { self.postMessage({ error: error instanceof Error ? error.message : '文件无法解析。' }); }
};
