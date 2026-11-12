import { SHA256 } from './sha256';
export type FileHashes = { sha256: string; parts: { part: number; size: number; sha256: string }[] };
export type HashMessage = { kind: 'progress'; bytes: number } | { kind: 'done'; result: FileHashes } | { kind: 'error'; message: string };
self.onmessage = async (event: MessageEvent<{ file: File }>) => {
  const { file } = event.data, hash = new SHA256(), parts: FileHashes['parts'] = [];
  try {
    for (let offset = 0; offset < file.size; offset += 32 * 1024 * 1024) {
      const data = await file.slice(offset, offset + 32 * 1024 * 1024).arrayBuffer();
      hash.update(new Uint8Array(data));
      const sha256 = [...new Uint8Array(await crypto.subtle.digest('SHA-256', data))].map(n => n.toString(16).padStart(2, '0')).join('');
      parts.push({ part: parts.length + 1, size: data.byteLength, sha256 });
      self.postMessage({ kind: 'progress', bytes: offset + data.byteLength } satisfies HashMessage);
    }
    self.postMessage({ kind: 'done', result: { sha256: hash.digest(), parts } } satisfies HashMessage);
  } catch (error) { self.postMessage({ kind: 'error', message: error instanceof Error ? error.message : '文件摘要计算失败。' } satisfies HashMessage); }
};
