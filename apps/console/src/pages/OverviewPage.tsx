import { StatusCard } from '@infrapro/ui';
import { useServiceHealth } from '../features/health/use-service-health';

export function OverviewPage() {
  const state = useServiceHealth();
  return (
    <>
      <StatusCard
        phase={state.phase}
        message={state.message}
        checkedAt={state.health?.time}
        details={[
          { label: '服务名称', value: state.health?.service ?? '等待响应' },
          { label: '服务版本', value: state.health?.version ?? '等待响应' },
        ]}
        onRefresh={state.refresh}
      />
      <div className="info-grid">
        <section className="info-card"><span className="info-index">01 / APPLICATION</span><h2>独立管理入口</h2><p>管理端已就绪。本页用于确认工程运行链路，业务页面将在后续迭代中加入。</p></section>
        <section className="info-card"><span className="info-index">02 / CONNECTION</span><h2>实时连接验证</h2><p>刷新状态会重新请求服务。若连接失败，页面会给出提示，并支持重新连接。</p></section>
      </div>
    </>
  );
}
