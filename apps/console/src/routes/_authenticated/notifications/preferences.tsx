import { createFileRoute } from '@tanstack/react-router';
import { NotificationPreferencesPage } from '../../../features/notifications/preferences';
export const Route = createFileRoute('/_authenticated/notifications/preferences')({ component: NotificationPreferencesPage });
