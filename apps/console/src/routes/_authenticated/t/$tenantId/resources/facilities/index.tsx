import { createFileRoute } from '@tanstack/react-router';
import { FacilitiesPage } from '../../../../../../features/resources/list-page';
export const Route = createFileRoute('/_authenticated/t/$tenantId/resources/facilities/')({ component: FacilitiesPage });
