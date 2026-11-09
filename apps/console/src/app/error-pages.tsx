import { Link } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import { AuthCard, AuthLayout, PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { WorkspaceShell } from './workspace-shell';
import { brand } from '../config/brand';

export function NotFoundPage() {
  const { account } = useIdentity();
  const action = <Button asChild variant="outline"><Link to="/">返回首页</Link></Button>;
  return account ? <WorkspaceShell><PageHeader title="页面未找到" description="这个地址对应的页面不存在。" />{action}</WorkspaceShell> : <AuthLayout brand={brand}><AuthCard title="页面未找到" description="请检查地址或返回首页。">{action}</AuthCard></AuthLayout>;
}

export function RouteErrorPage() {
  const { account } = useIdentity();
  const content = <div className="space-y-4 text-center"><ErrorState title="页面暂时无法加载" message="请检查连接后重试，或返回首页。" retryLabel="重新加载页面" onRetry={() => window.location.reload()} /><Button asChild variant="outline"><a href={import.meta.env.BASE_URL}>返回首页</a></Button></div>;
  return account ? <WorkspaceShell>{content}</WorkspaceShell> : <AuthLayout brand={brand}>{content}</AuthLayout>;
}
