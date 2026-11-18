import type { AnalysisRunInput } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Field } from '../monitoring/common';
import { formatNS } from '../monitoring/format';
import { RangeFields } from './channel-controls';
import { isoNS } from './common';

type Period = AnalysisRunInput['output_range'];
const halfHour = 1_800_000_000_000n;
const day = 86_400_000_000_000n;
const utcPlusEight = day / 3n;

function pickerValue(ns: string | null | undefined) {
  if (!ns || !/^-?\d+$/.test(ns)) return '';
  return formatNS(BigInt(ns) + utcPlusEight).slice(0, 19);
}

function fromPicker(value: string) {
  if (!value) return 'invalid:';
  return isoNS((value.length === 16 ? value + ':00' : value) + '+08:00');
}

export function AnalysisPeriod({value, original, onChange}: {value: Period; original?: Period; onChange: (v: Period) => void}) {
  const recent = (duration: bigint) => {
    const end = BigInt(Date.now()) * 1_000_000n;
    onChange({from_ns: (end - duration).toString(), to_ns: end.toString()});
  };
  const start = original ? BigInt(original.from_ns) : 0n;
  const end = original?.to_ns ? BigInt(original.to_ns) : start;
  return <fieldset className="min-w-0 space-y-3">
    <legend className="text-sm font-medium">分析时段（UTC+08:00）</legend>
    <div className="flex flex-wrap gap-2">
      {original ? <>
        <Button type="button" size="sm" variant="outline" onClick={() => onChange(original)}>原时段</Button>
        {end - start > halfHour && <>
          <Button type="button" size="sm" variant="outline" onClick={() => onChange({from_ns: original.from_ns, to_ns: (start + halfHour).toString()})}>前半小时</Button>
          <Button type="button" size="sm" variant="outline" onClick={() => onChange({from_ns: (end - halfHour).toString(), to_ns: original.to_ns})}>后半小时</Button>
        </>}
      </> : <>
        <Button type="button" size="sm" variant="outline" onClick={() => recent(halfHour)}>近半小时</Button>
        <Button type="button" size="sm" variant="outline" onClick={() => recent(day)}>近 24 小时</Button>
      </>}
    </div>
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label="开始日期时间（含）" type="datetime-local" step="1" value={pickerValue(value.from_ns)}
        onChange={v => onChange({...value, from_ns: fromPicker(v)})}/>
      <Field label="结束日期时间（不含）" type="datetime-local" step="1" value={pickerValue(value.to_ns)}
        onChange={v => onChange({...value, to_ns: fromPicker(v)})}/>
    </div>
    <details><summary className="min-h-10 cursor-pointer py-2 text-sm text-muted-foreground">精确输入时段</summary>
      <RangeFields label="纳秒级时段" value={value} onChange={onChange}/>
    </details>
  </fieldset>;
}
