import { createFileRoute, redirect } from '@tanstack/react-router';
import { WorkspaceShell } from '../../app/workspace-shell';
import { relativePath, safeReturn } from '../../config/navigation';

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: ({ context, location }) => { if (context.identity.phase === 'anonymous') throw redirect({ to: '/login', search: { returnTo: safeReturn(relativePath(location.pathname)) }, replace: true }); },
  component: WorkspaceShell,
});
