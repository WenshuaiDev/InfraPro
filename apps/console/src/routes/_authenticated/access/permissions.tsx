import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/access/permissions')({
  beforeLoad: () => { throw redirect({ to: '/access/roles', replace: true }); },
});
