import { createFileRoute, redirect, retainSearchParams } from '@tanstack/react-router';
import { WorkspaceShell } from '../../app/workspace-shell';
import { relativePath, safeReturn } from '../../config/navigation';

export const Route = createFileRoute('/_authenticated')({
  validateSearch: (search: Record<string, unknown>): { variant?: 'A'; picker?: 'tenant' | 'project' } => import.meta.env.DEV && import.meta.env.VITE_NAVIGATION_PROTOTYPE === '1' ? { ...(search.variant === 'A' ? { variant: search.variant } : {}), ...(search.picker === 'tenant' || search.picker === 'project' ? { picker: search.picker } : {}) } : {},
  search: { middlewares: [retainSearchParams(['variant'])] },
  beforeLoad: ({ context, location }) => { if (context.identity.phase === 'anonymous') throw redirect({ to: '/login', search: { returnTo: safeReturn(relativePath(location.pathname)) }, replace: true }); },
  component: WorkspaceShell,
});
