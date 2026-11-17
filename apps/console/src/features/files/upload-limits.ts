export function uploadSizeLabel(bytes: string | number) {
  const value = Number(bytes);
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
  const index = Math.min(units.length - 1, Math.max(0, Math.floor(Math.log2(value) / 10)));
  return `${Number((value / 1024 ** index).toFixed(2))} ${units[index]}`;
}
