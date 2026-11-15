import { useEffect, useState } from 'react';
import { Link, useLocation, useRouter } from '@tanstack/react-router';
import { ArrowLeft, PanelsTopLeft } from 'lucide-react';
import { useIdentity } from '@infrapro/api-client/react';
import type { NavigationGroup } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { allowedPath, safeReturn, workspaceScope } from '../config/navigation';
import { useWorkspacePicker } from './workspace-picker-context';

type ReturnTarget = { href: string; label: string };
const storageKey = (accountId: string) => `infrapro:workspace-return:${accountId}`;

function targetURL(href: unknown) {
  if (typeof href !== 'string' || href.length > 16_384 || !href.startsWith('/') || href.startsWith('//') || href.includes('\\') || Array.from(href).some(char => char.charCodeAt(0) < 32)) return;
  const url = new URL(href, 'https://infrapro.invalid');
  const path = url.pathname.replace(/\/$/, '') || '/';
  if (url.origin !== 'https://infrapro.invalid' || safeReturn(path) !== path || workspaceScope(path).kind === 'account') return;
  return url;
}

function readTarget(accountId: string): ReturnTarget | undefined {
  try {
    const value: unknown = JSON.parse(sessionStorage.getItem(storageKey(accountId)) ?? 'null');
    if (!value || typeof value !== 'object' || !('href' in value) || !('label' in value)) return;
    if (typeof value.href === 'string' && targetURL(value.href) && typeof value.label === 'string' && value.label.length <= 500) return { href: value.href, label: value.label };
  } catch { /* Navigation still works when browser storage is unavailable. */ }
}

// The shell owns this state above the layout switch, so navigation still works
// without sessionStorage. Global pages never replace the last business location.
// eslint-disable-next-line react-refresh/only-export-components
export function useWorkspaceReturn({ path, groups, workspaceName }: { path: string; groups: NavigationGroup[]; workspaceName: string }) {
  const { account, can } = useIdentity();
  const location = useLocation();
  const accountId = account!.id;
  const [target, setTarget] = useState(() => readTarget(accountId));
  const kind = workspaceScope(path).kind;
  const pageName = groups.flatMap(group => group.items.flatMap(item => 'items' in item ? item.items : [item])).find(item => item.active)?.title;
  const href = `${path}${location.searchStr}${location.hash ? `#${location.hash}` : ''}`;
  const label = pageName ? `${workspaceName} · ${pageName}` : workspaceName;

  if (kind !== 'account' && targetURL(href) && allowedPath(path, can) && (target?.href !== href || target.label !== label)) setTarget({ href, label });
  useEffect(() => {
    if (!target) return;
    try { sessionStorage.setItem(storageKey(accountId), JSON.stringify(target)); } catch { /* Retain the in-memory return target. */ }
  }, [accountId, target]);

  const url = target && targetURL(target.href);
  return url && allowedPath(url.pathname, can) ? target : undefined;
}

export function WorkspaceReturn({ target }: { target?: ReturnTarget }) {
  const router = useRouter();
  const picker = useWorkspacePicker();
  const url = target && targetURL(target.href);
  if (!url && picker.summary?.mode === 'none') return null;
  return <nav aria-label="工作台返回导航" className="mb-4 flex min-w-0 items-center gap-3">
    {url ? <>
      <Button asChild variant="outline" size="sm" className="min-h-11 shrink-0">
        <Link to={url.pathname} search={router.options.parseSearch!(url.search)} hash={url.hash.slice(1)} preload={false} aria-label={`返回工作台：${target.label}`}>
          <ArrowLeft aria-hidden="true" />返回工作台
        </Link>
      </Button>
      <span className="min-w-0 truncate text-sm text-muted-foreground" title={target.label}>{target.label}</span>
    </> : <Button variant="outline" size="sm" className="min-h-11" disabled={picker.pending} onClick={() => picker.open()}><PanelsTopLeft aria-hidden="true" />选择工作空间</Button>}
  </nav>;
}
