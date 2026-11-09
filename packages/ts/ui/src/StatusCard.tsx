type StatusCardProps = {
  phase: 'loading' | 'success' | 'error';
  message?: string;
  checkedAt?: string;
  details: { label: string; value: string }[];
  onRefresh: () => void;
};

export function StatusCard({ phase, message, checkedAt, details, onRefresh }: StatusCardProps) {
  const pending = phase === 'loading';
  const title = pending ? '正在检查连接' : phase === 'success' ? '服务连接正常' : '服务连接失败';
  const description = pending ? '正在获取服务的最新状态…'
    : phase === 'success' ? '已收到 API 的实时响应。' : message;
  return (
    <section className={`status-card status-${phase}`} aria-label="服务状态">
      <div className="status-card-top">
        <div className="status-heading" role="status" aria-live="polite" aria-atomic="true">
          <span className="status-indicator" aria-hidden="true"><span /></span>
          <div><p className="eyebrow">API CONNECTION</p><h2>{title}</h2><p className="status-description">{description}</p></div>
        </div>
        <button className="refresh-button" type="button" disabled={pending} onClick={onRefresh}>
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5M5.5 8a7 7 0 0 1 11.6-2L20 9M4 15l2.9 3A7 7 0 0 0 18.5 16" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" /></svg>
          {pending ? '检查中…' : phase === 'error' ? '重新连接' : '刷新状态'}
        </button>
      </div>
      <dl className="status-details">
        {details.map(({ label, value }) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}
        <div><dt>最近响应</dt><dd>{checkedAt ? <time dateTime={checkedAt}>{new Date(checkedAt).toLocaleString('zh-CN', { hour12: false })}</time> : '等待响应'}</dd></div>
      </dl>
      <p className="status-footnote">状态来自本次请求；点击刷新可重新检查。</p>
    </section>
  );
}
