import { createFileRoute } from '@tanstack/react-router';
export const Route = createFileRoute('/_authenticated/t/$tenantId/resources/facilities/$facilityId')({ component: () => null });
