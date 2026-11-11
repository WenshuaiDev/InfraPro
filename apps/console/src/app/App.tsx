import { useEffect, useState } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import { IdentityProvider, useIdentity } from '@infrapro/api-client/react';
import { AuthLayout } from '@infrapro/layouts';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ThemeProvider } from '@infrapro/ui/theme';
import { Toaster } from '@infrapro/ui/components/sonner';
import { brand } from '../config/brand';
import { makeRouter } from './router';
import { observeCredentialIdentity } from '../features/auth/credential';

function Unavailable() {
  const { reload } = useIdentity();
  return <AuthLayout brand={brand}><ErrorState title="服务暂时不可用" message="暂时无法确认账号状态，请稍后重试。" retryLabel="重新连接账号服务" onRetry={() => void reload()} /></AuthLayout>;
}

function Runtime() {
  const identity = useIdentity();
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 15_000, refetchOnWindowFocus: false }, mutations: { retry: false } } }));
  const [router] = useState(() => makeRouter({ identity, queryClient }));
  useEffect(() => () => { void queryClient.cancelQueries(); queryClient.clear(); }, [queryClient]);
  return <QueryClientProvider client={queryClient}>
    {identity.phase === 'unavailable' && <Unavailable />}
    <div hidden={identity.phase === 'unavailable'}><RouterProvider router={router} context={{ identity, queryClient }} /></div>
  </QueryClientProvider>;
}

function Bootstrap() {
  const { help, phase, account } = useIdentity();
  useEffect(() => { if (phase === 'authenticated' || phase === 'anonymous') observeCredentialIdentity(account?.id); }, [phase, account?.id]);
  if (!help) return phase === 'unavailable' ? <Unavailable /> : <AuthLayout brand={brand}><LoadingState label="正在确认登录状态…" /></AuthLayout>;
  return <Runtime />;
}

export function App() {
  return <ThemeProvider><IdentityProvider baseUrl={import.meta.env.VITE_API_BASE_URL || '/api/v1'}><Bootstrap /></IdentityProvider><Toaster /></ThemeProvider>;
}
