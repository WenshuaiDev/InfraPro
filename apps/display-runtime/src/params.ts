/** Structured-clone messages still need a bounded JSON boundary before fetch. */
export function validParams(value: unknown): boolean {
  if (value !== undefined && (value === null || typeof value !== "object" || Array.isArray(value))) return false;
  try {
    return new TextEncoder().encode(JSON.stringify(value ?? {})).byteLength <= 262144;
  } catch { return false; }
}
