import { createFileRoute } from '@tanstack/react-router';
import { ProfilePage } from '../../features/account/profile-page';

export const Route = createFileRoute('/_authenticated/profile')({ component: ProfilePage });
