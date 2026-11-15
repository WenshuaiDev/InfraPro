import { createFileRoute, redirect } from '@tanstack/react-router';
import { importSearch } from '../../../../../../features/monitoring/import-search';
export const Route = createFileRoute('/_authenticated/t/$tenantId/monitoring/batches/$batchId')({
  validateSearch: importSearch,
  beforeLoad: ({ params, search }) => { throw redirect({ to: '/t/$tenantId/monitoring/imports/$batchId', params, search, replace: true }); },
});
