import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './features/auth/credential';
import { App } from './app/App';
import { initializeTheme } from '@infrapro/ui/theme-init';
import './styles/global.css';

initializeTheme();

const root = document.getElementById('root');
if (!root) throw new Error('Application root was not found.');
createRoot(root).render(<StrictMode><App /></StrictMode>);
