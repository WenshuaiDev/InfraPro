import type { MonitoringTrendSeries } from '@infrapro/api-client';
import { parseNS } from './format';
import { observationTime, plotCoefficient, type DetailRow } from './detail-format';

// Join adjacent raw tiles only when neither reports a gap. Summary buckets and
// empty or discontinuous intervals never imply an observed connection.
export function plotObservations(series: MonitoringTrendSeries): (DetailRow | null)[] {
  return series.buckets.flatMap((bucket, index) => {
    const previous = series.buckets[index - 1];
    const contiguous = !series.summary && !bucket.discontinuous && previous && !previous.discontinuous && previous.points.length > 0 && bucket.points.length > 0 && previous.range?.to && bucket.range?.from && parseNS(previous.range.to) === parseNS(bucket.range.from);
    const points = [...bucket.points].sort((a, b) => parseNS(observationTime(a)!) < parseNS(observationTime(b)!) ? -1 : parseNS(observationTime(a)!) > parseNS(observationTime(b)!) ? 1 : 0);
    return [...(index && !contiguous ? [null] : []), ...points.flatMap(point => series.summary || bucket.discontinuous ? [null, point] : [point])];
  });
}
export function plotBaseline(first?: Pick<DetailRow, 'value'>, precision = 12) {
  const coefficient = first?.value != null ? plotCoefficient(first.value, precision) : 0n;
  // Normal engineering magnitudes use their actual values; large decimals retain
  // exact subtraction before conversion so tiny changes remain distinguishable.
  return coefficient > BigInt(Number.MAX_SAFE_INTEGER) || coefficient < -BigInt(Number.MAX_SAFE_INTEGER) ? coefficient : 0n;
}
