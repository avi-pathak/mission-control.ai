import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const httpTarget = env.MC_PROXY_TARGET || 'http://localhost:8080';
  const wsTarget = httpTarget.replace(/^http/, 'ws');
  return {
    plugins: [react()],
    server: {
      port: 5173,
      proxy: {
        '/api': { target: httpTarget, changeOrigin: true },
        '/ws': { target: wsTarget, ws: true },
      },
    },
  };
});
