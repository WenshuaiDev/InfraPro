import { Check, CircleAlert, CircleCheck, FlaskConical, PencilLine, RotateCcw } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { bounds, example, exampleLabel, initialTrial, shapes, unitRef, type RecordView, type Trial } from './model';

export function SampleBench({ view, trial, change }: { view: RecordView; trial: Trial; change: (next: Trial) => void }) {
  const { rule } = view, { raw, unit } = trial;
  const isEnum = rule.shape === 'enum';
  const numeric = Number(raw), hasValue = raw.trim() !== '';
  const shapeOK = isEnum ? hasValue : hasValue && /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(raw.trim()) && Number.isFinite(numeric) && Math.abs(numeric) <= Number.MAX_SAFE_INTEGER && (rule.shape !== 'integer' || Number.isSafeInteger(numeric));
  const limitsOK = isEnum ? rule.options?.some(option => option.key === raw) : shapeOK && (rule.minimum == null || numeric >= rule.minimum) && (rule.maximum == null || numeric <= rule.maximum);
  const unitOK = isEnum ? unit === '' : !!unit && rule.allowed_units?.includes(unit);
  const missingOptions = isEnum && !rule.options?.length;
  const checks = [
    { label: '取值形态', note: isEnum ? '单个状态编码' : rule.shape === 'integer' ? '一个有效整数' : '一个有效数值', ok: shapeOK },
    { label: isEnum ? '选项成员' : '数值边界', note: isEnum ? (missingOptions ? '本定义尚未配置选项' : `须属于 ${rule.options.length} 个已定义选项`) : bounds(rule), ok: limitsOK },
    { label: '记录单位', note: isEnum ? '枚举不带物理单位' : `须使用 ${rule.allowed_units?.length ?? 0} 个允许单位版本之一`, ok: unitOK },
  ];
  const pass = checks.every(check => check.ok);
  const invalid = () => {
    if (isEnum) change({ raw: '__outside_options__', unit: '' });
    else if (rule.minimum != null) change({ ...trial, raw: String(rule.minimum - 1) });
    else if (rule.maximum != null) change({ ...trial, raw: String(rule.maximum + 1) });
    else if (rule.shape === 'integer') change({ ...trial, raw: '1.5' });
    else change({ raw: example(view), unit: '__other_unit__' });
  };
  return <div className="mc-bench"><section className="mc-bench-input"><p className="mc-overline"><PencilLine />填写一条样例</p>
    <div className="mc-reading"><span>{exampleLabel(view, raw)}</span>{!isEnum && <small>{unit === '__other_unit__' ? '其他单位' : unitRef(view, unit).symbol}</small>}</div>
    <p className="mc-muted">{view.content.name} · {shapes[rule.shape]} · 非现场读数</p>
    <div className="mc-form-grid">{isEnum ? <label>样例状态<select value={raw} onChange={event => change({ raw: event.target.value, unit: '' })}>
      <option value="">请选择状态</option>{rule.options?.map(option => <option value={option.key} key={option.key}>{option.name} · {option.key}</option>)}<option value="__outside_options__">未定义的状态（反例）</option></select></label>
    : <><label>样例值<input value={raw} inputMode="decimal" maxLength={32} onChange={event => change({ ...trial, raw: event.target.value })} /></label>
      <label>样例单位<select value={unit} onChange={event => change({ ...trial, unit: event.target.value })}><option value="">请选择单位</option>
        {rule.allowed_units?.map(id => { const ref = unitRef(view, id); return <option key={id} value={id}>{ref.name}{ref.symbol && `（${ref.symbol}）`}{ref.number && ` · V${ref.number}`}</option>; })}<option value="__other_unit__">不在允许列表中（反例）</option></select></label></>}
    </div><div className="mc-inline-actions"><Button variant="outline" size="sm" onClick={invalid}><FlaskConical />试一个反例</Button><Button variant="ghost" size="sm" onClick={() => change(initialTrial(view))}><RotateCcw />还原样例</Button></div>
  </section><section className="mc-bench-result" aria-live="polite"><p className="mc-overline">本地规则核对</p>
    <h3 className="mc-verdict" data-pass={hasValue && pass}>{hasValue && pass ? <CircleCheck /> : <CircleAlert />}{missingOptions ? '尚未配置枚举选项' : !hasValue ? '等待填写样例' : pass ? '符合取值规则' : '不符合取值规则'}</h3>
    <ul className="mc-checks">{checks.map(check => <li key={check.label} data-ok={!!check.ok}><span className="mc-check-icon">{check.ok ? <Check /> : <span aria-hidden="true">—</span>}</span><div><strong>{check.label}</strong><p>{check.note}</p></div><small>{check.ok ? '符合' : '未满足'}</small></li>)}</ul>
    <p className="mc-note">仅核对形态、边界与单位；不换算、不写入观测，也不代表正式发布校验。</p>
  </section></div>;
}
