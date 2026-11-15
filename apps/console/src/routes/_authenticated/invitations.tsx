import { createFileRoute } from '@tanstack/react-router';
import { InvitationsPage } from '../../features/account/invitations-page';
export const Route = createFileRoute('/_authenticated/invitations')({validateSearch: (s: Record<string, unknown>): { invitation?: string } => ({ invitation: typeof s.invitation === 'string' && /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i.test(s.invitation) ? s.invitation : undefined }), component: function InvitationsRoute() { const { invitation } = Route.useSearch(); return <InvitationsPage invitationID={invitation} />; } });
