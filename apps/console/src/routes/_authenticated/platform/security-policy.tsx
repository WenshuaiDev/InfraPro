import { createFileRoute } from '@tanstack/react-router';
import { SecurityPolicyPage } from '../../../features/account/security-policy-page';
export const Route = createFileRoute('/_authenticated/platform/security-policy')({ component: SecurityPolicyPage });
