import { createFileRoute } from '@tanstack/react-router';
import { InvitationsPage } from '../../features/account/invitations-page';

export const Route = createFileRoute('/_authenticated/invitations')({ component: InvitationsPage });
