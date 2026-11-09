import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const port = Number(env.SCREEN_PORT ?? 5174);
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('Invalid SCREEN_PORT');
  const proxy = { '/api': { target: env.API_PROXY_TARGET || 'http://127.0.0.1:8080', changeOrigin: true } };
  return {
    plugins: [react()],
    server: { host: '127.0.0.1', port, strictPort: true, proxy },
    preview: { host: '127.0.0.1', port, strictPort: true, proxy },
  };
});
