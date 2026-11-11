import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/t/$tenantId/permissions')({
  beforeLoad: ({ params }) => { throw redirect({ to: '/t/$tenantId/roles', params: { tenantId: params.tenantId }, replace: true }); },
});
