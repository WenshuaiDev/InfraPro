import { AppFrame } from '@infrapro/ui';
import { OverviewPage } from '../pages/OverviewPage';

export function App() {
  return (
    <AppFrame
      variant="console"
      title="工程运行总览"
      description="查看应用与服务的连接状态，确认开发环境已准备就绪。"
      otherAppUrl={import.meta.env.VITE_OTHER_APP_URL || 'http://127.0.0.1:5174'}
    >
      <OverviewPage />
    </AppFrame>
  );
}
