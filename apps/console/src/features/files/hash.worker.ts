import { SHA256 } from './sha256';
export type FileHashes = { sha256: string; parts: { part: number; size: number; sha256: string }[] };
export type HashMessage = { kind: 'progress'; bytes: number } | { kind: 'done'; result: FileHashes } | { kind: 'error'; message: string };
self.onmessage = async (event: MessageEvent<{ file: File; partBytes?: number }>) => {
  const { file, partBytes = 32 * 1024 * 1024 } = event.data, hash = new SHA256(), parts: FileHashes['parts'] = [];
  try {
    if (!Number.isSafeInteger(partBytes) || partBytes < 1 || partBytes > 32 * 1024 * 1024) throw Error("无效分片大小");
    for (let offset = 0; offset < file.size; offset += partBytes) {
      const data = await file.slice(offset, offset + partBytes).arrayBuffer();
      hash.update(new Uint8Array(data));
      const partHash = new SHA256(); partHash.update(new Uint8Array(data)); const sha256 = partHash.digest();
      parts.push({ part: parts.length + 1, size: data.byteLength, sha256 });
      self.postMessage({ kind: 'progress', bytes: offset + data.byteLength } satisfies HashMessage);
    }
    self.postMessage({ kind: 'done', result: { sha256: hash.digest(), parts } } satisfies HashMessage);
  } catch (error) { self.postMessage({ kind: 'error', message: error instanceof Error ? error.message : '文件摘要计算失败。' } satisfies HashMessage); }
};
