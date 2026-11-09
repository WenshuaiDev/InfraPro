import { createFileRoute } from '@tanstack/react-router';
import { LoginPage } from '../../features/auth/login-page';
import { loginSearchSchema } from '../../features/auth/forms';

export const Route = createFileRoute('/_auth/login')({ validateSearch: loginSearchSchema, component: LoginPage });
