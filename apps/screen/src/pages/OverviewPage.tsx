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
        <section className="info-card"><span className="info-index">01 / APPLICATION</span><h2>独立展示入口</h2><p>展示端拥有独立的运行入口和构建产物，可随着业务需求扩展为专用展示页面。</p></section>
        <section className="info-card"><span className="info-index">02 / CONNECTION</span><h2>真实状态来源</h2><p>本页展示当前 API 进程的响应，不包含模拟设备数据或业务监测结果。</p></section>
      </div>
    </>
  );
}
