import { createFileRoute } from '@tanstack/react-router';
import { NotificationPage } from '../../../features/notifications/center';
import { notificationSearch } from '../../../features/notifications/search';
export const Route = createFileRoute('/_authenticated/notifications/$notificationId')({ validateSearch: notificationSearch, component: function Page() { const { notificationId } = Route.useParams(); return <NotificationPage filter={Route.useSearch()} id={notificationId} />; } });
