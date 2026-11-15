import { createFileRoute } from '@tanstack/react-router';
import { NotificationCenter } from '../../../features/notifications/center';
import { notificationSearch } from '../../../features/notifications/search';
export const Route = createFileRoute('/_authenticated/notifications/$notificationId')({ validateSearch: notificationSearch, component: function Page() { const { notificationId } = Route.useParams(); return <NotificationCenter filter={Route.useSearch()} selectedId={notificationId} />; } });
