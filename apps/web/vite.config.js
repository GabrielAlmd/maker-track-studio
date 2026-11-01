import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import { configDefaults } from 'vitest/config'
import process from 'node:process'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  return {
    server: {
      host: true,
      proxy: {
        '/api': {
          target: env.VITE_API_URL || 'http://127.0.0.1:8080',
          rewrite: (path) => path.replace(/^\/api/, ''),
        },
      },
    },
    test: {
      environment: 'jsdom',
      setupFiles: './src/test/setupTests.js',
      globals: true,
      exclude: [...configDefaults.exclude, 'visual/**'],
    },
    plugins: [react()],
  };
})
