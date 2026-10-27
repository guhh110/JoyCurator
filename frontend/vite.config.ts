import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  plugins: [vue()],
  cacheDir: '../local/cache/vite',
  server: {
    host: '127.0.0.1',
    port: 5175,
    strictPort: true,
    proxy: { '/api': { target: 'http://127.0.0.1:8081', changeOrigin: false } },
  },
  build: { outDir: '../local/build/frontend', emptyOutDir: true },
  test: { include: ['src/**/*.test.ts'] },
})
