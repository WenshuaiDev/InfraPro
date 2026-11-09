import { createFileRoute, Outlet } from '@tanstack/react-router';
import { AuthLayout } from '@infrapro/layouts';
import { brand } from '../../config/brand';

export const Route = createFileRoute('/_auth')({ component: () => <AuthLayout brand={brand}><Outlet /></AuthLayout> });
