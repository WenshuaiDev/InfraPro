import { createFileRoute } from '@tanstack/react-router';
import { LoginPage } from '../../features/auth/login-page';
export const Route = createFileRoute('/_auth/mfa-recovery')({ component: () => <LoginPage recovery /> });
