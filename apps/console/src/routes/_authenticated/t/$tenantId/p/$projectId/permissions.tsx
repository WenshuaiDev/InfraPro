import { createFileRoute, redirect } from '@tanstack/react-router';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/permissions')({ beforeLoad: ({ params }) => { throw redirect({ to: '/t/$tenantId/p/$projectId/roles', params, replace: true }); } });
