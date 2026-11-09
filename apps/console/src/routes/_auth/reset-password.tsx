import { createFileRoute } from '@tanstack/react-router';
import { CredentialPage } from '../../features/auth/credential-page';

export const Route = createFileRoute('/_auth/reset-password')({ component: () => <CredentialPage reset /> });
