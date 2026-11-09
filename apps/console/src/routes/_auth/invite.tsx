import { createFileRoute } from '@tanstack/react-router';
import { CredentialPage } from '../../features/auth/credential-page';

export const Route = createFileRoute('/_auth/invite')({ component: CredentialPage });
