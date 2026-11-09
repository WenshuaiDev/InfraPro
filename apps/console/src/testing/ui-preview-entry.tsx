import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { ThemeProvider } from '@infrapro/ui/theme';
import { initializeTheme } from '@infrapro/ui/theme-init';
import { Preview } from './ui-preview';

initializeTheme();
const root = document.getElementById('root');
if (root) createRoot(root).render(<StrictMode><ThemeProvider><Preview /></ThemeProvider></StrictMode>);
