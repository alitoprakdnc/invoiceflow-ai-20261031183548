import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react-swc';
import path from 'path';

export default defineConfig(({ mode }) => {
  // Load all env variables (including those without the VITE_ prefix)
  const env = loadEnv(mode, process.cwd(), '');

  // Backend URL for API proxy – can be overridden via VITE_BACKEND_URL
  const backendUrl = env.VITE_BACKEND_URL ?? 'http://localhost:8080';

  return {
    plugins: [react()],

    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src'),
      },
    },

    server: {
      port: 3000,
      open: true,
      proxy: {
        // Proxy all API calls to the Go backend
        '/api': {
          target: backendUrl,
          changeOrigin: true,
          secure: false,
          ws: true,
        },
        // Proxy WebSocket connections used for real‑time status updates
        '/ws': {
          target: backendUrl,
          ws: true,
          changeOrigin: true,
          secure: false,
        },
      },
    },

    preview: {
      port: 3000,
    },

    build: {
      outDir: 'dist',
      sourcemap: false,
      rollupOptions: {
        output: {
          // Split vendor code to improve caching
          manualChunks: {
            vendor: [
              'react',
              'react-dom',
              '@tanstack/react-query',
              'axios',
              'chart.js',
            ],
          },
        },
      },
    },

    define: {
      // Expose the app version at compile time
      __APP_VERSION__: JSON.stringify(env.npm_package_version ?? '0.0.0'),
    },

    optimizeDeps: {
      include: ['react', 'react-dom'],
    },
  };
});