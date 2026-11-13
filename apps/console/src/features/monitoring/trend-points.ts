import type { MonitoringObservation, MonitoringTrendSeries } from '@infrapro/api-client';
import { decimalCoefficient, parseNS } from './format';

// Join adjacent raw tiles only when neither reports a gap. Summary buckets and
// empty or discontinuous intervals never imply an observed connection.
export function plotObservations(series: MonitoringTrendSeries): (MonitoringObservation | null)[] {
  return series.buckets.flatMap((bucket, index) => {
    const previous = series.buckets[index - 1];
    const contiguous = !series.summary && !bucket.discontinuous && previous && !previous.discontinuous && previous.points.length > 0 && bucket.points.length > 0 && previous.range?.to && bucket.range?.from && parseNS(previous.range.to) === parseNS(bucket.range.from);
    const points = [...bucket.points].sort((a, b) => parseNS(a.observed_at) < parseNS(b.observed_at) ? -1 : parseNS(a.observed_at) > parseNS(b.observed_at) ? 1 : 0);
    return [...(index && !contiguous ? [null] : []), ...points.flatMap(point => series.summary || bucket.discontinuous ? [null, point] : [point])];
  });
}
export function plotBaseline(first?: MonitoringObservation) {
  const coefficient = first ? decimalCoefficient(first.value) : 0n;
  // Normal engineering magnitudes use their actual values; large decimals retain
  // exact subtraction before conversion so tiny changes remain distinguishable.
  return coefficient > BigInt(Number.MAX_SAFE_INTEGER) || coefficient < -BigInt(Number.MAX_SAFE_INTEGER) ? coefficient : 0n;
}
