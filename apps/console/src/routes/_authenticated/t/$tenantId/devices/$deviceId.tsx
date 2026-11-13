import { createFileRoute } from '@tanstack/react-router';
export const Route = createFileRoute('/_authenticated/t/$tenantId/devices/$deviceId')({ component: () => null });
