import { createFileRoute } from '@tanstack/react-router';
import { NotificationCenter } from '../../../features/notifications/center';
import { notificationSearch } from '../../../features/notifications/search';
export const Route = createFileRoute('/_authenticated/notifications/')({ validateSearch: notificationSearch, component: function Page() { return <NotificationCenter filter={Route.useSearch()} />; } });
