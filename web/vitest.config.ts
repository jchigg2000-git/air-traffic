import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Test-only config (kept separate from vite.config.ts so `vite build` is untouched).
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})
