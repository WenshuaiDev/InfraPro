const dateTime = new Intl.DateTimeFormat('zh-CN', { timeZone:'Asia/Shanghai', year:'numeric', month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit', second:'2-digit', hourCycle:'h23' });
const dateOnly = new Intl.DateTimeFormat('zh-CN', { timeZone:'Asia/Shanghai', year:'numeric', month:'2-digit', day:'2-digit' });

export function reportTime(ns?: string | null, format?: 'date') {
  if (!ns) return '不限';
  const at = BigInt(ns), second = at < 0n ? (at - 999999999n) / 1000000000n : at / 1000000000n;
  const date = new Date(Number(second * 1000n));
  if (format === 'date') return dateOnly.format(date);
  const fraction = (at - second * 1000000000n).toString().padStart(9, '0').replace(/0+$/, '');
  return dateTime.format(date) + (fraction ? '.' + fraction : '');
}

export function inputStateLabel(state: string) {
  if (state === 'current') return '依据未变化';
  if (state === 'affected') return '输入已有变化';
  return '原依据包含撤销';
}

export function sampleOffset(sample: string, output: string) {
  const delta = BigInt(sample) - BigInt(output);
  if (delta === 0n) return '与输出同时';
  const distance = delta < 0n ? -delta : delta;
  const fraction = (distance % 1000000000n).toString().padStart(9, '0').replace(/0+$/, '');
  return (delta < 0n ? '早于输出 ' : '晚于输出 ') + (distance / 1000000000n).toString() + (fraction ? '.' + fraction : '') + ' 秒';
}
