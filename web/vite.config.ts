import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Air-Traffic web SPA — dev on 5202, proxy API + synthetic surfaces to the Go server on 8122.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5202,
    strictPort: true,
    proxy: {
      '/api': 'http://127.0.0.1:8122',
      '/synthetic': 'http://127.0.0.1:8122',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
