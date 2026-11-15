import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const port = Number(env.DIAGNOSTICS_WEB_PORT ?? 5196);
  const target = env.DIAGNOSTICS_PROXY_TARGET ?? 'http://127.0.0.1:8096';
  const url = new URL(target);
  if (!Number.isInteger(port) || port < 1 || port > 65535 || !['127.0.0.1', '[::1]', 'localhost'].includes(url.hostname) || url.protocol !== 'http:') throw new Error('Invalid diagnostic development listener');
  return { plugins: [react(), tailwindcss()], server: { host: '127.0.0.1', port, strictPort: true, proxy: { '/api/diagnostics': { target, changeOrigin: true } } }, build: { manifest: true, sourcemap: false } };
});
