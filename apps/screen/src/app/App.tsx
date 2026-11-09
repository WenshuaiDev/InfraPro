import { AppFrame } from '@infrapro/ui';
import { OverviewPage } from '../pages/OverviewPage';

export function App() {
  return (
    <AppFrame
      variant="screen"
      title="运行状态展示"
      description="以独立展示入口，查看当前服务的真实运行状态。"
      otherAppUrl={import.meta.env.VITE_OTHER_APP_URL || 'http://127.0.0.1:5173'}
    >
      <OverviewPage />
    </AppFrame>
  );
}
