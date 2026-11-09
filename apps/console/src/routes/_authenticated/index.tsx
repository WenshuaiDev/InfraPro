import { createFileRoute } from '@tanstack/react-router';
import { HomePage } from '../../features/account/home-page';

export const Route = createFileRoute('/_authenticated/')({ component: HomePage });
