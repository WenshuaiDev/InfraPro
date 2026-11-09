import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { tanstackRouter } from '@tanstack/router-plugin/vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const port = Number(env.CONSOLE_PORT ?? 5173);
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('Invalid CONSOLE_PORT');
  const proxy = { '/api': { target: env.API_PROXY_TARGET || 'http://127.0.0.1:8080', changeOrigin: true } };
  return {
    base: env.VITE_APP_BASE || '/',
    plugins: [tanstackRouter({ target: 'react', autoCodeSplitting: true }), react(), tailwindcss()],
    server: { host: '127.0.0.1', port, strictPort: true, proxy },
    preview: { host: '127.0.0.1', port, strictPort: true, proxy },
  };
});
