import path from 'node:path'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig, loadEnv } from 'vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, path.resolve(__dirname, '..'), '')

  return {
    envDir: '..',
    define: {
      'import.meta.env.APP_ENV': JSON.stringify(env.APP_ENV || 'local'),
      'import.meta.env.AUTH_PASSWORD_MIN_LENGTH': JSON.stringify(env.AUTH_PASSWORD_MIN_LENGTH || ''),
      'import.meta.env.AUTH_PASSWORD_REQUIRE_UPPER': JSON.stringify(env.AUTH_PASSWORD_REQUIRE_UPPER || ''),
      'import.meta.env.AUTH_PASSWORD_REQUIRE_LOWER': JSON.stringify(env.AUTH_PASSWORD_REQUIRE_LOWER || ''),
      'import.meta.env.AUTH_PASSWORD_REQUIRE_NUMBER': JSON.stringify(env.AUTH_PASSWORD_REQUIRE_NUMBER || ''),
      'import.meta.env.AUTH_PASSWORD_REQUIRE_SYMBOL': JSON.stringify(env.AUTH_PASSWORD_REQUIRE_SYMBOL || ''),
    },
    plugins: [vue(), tailwindcss()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      port: 5173,
      proxy: {
        '/api': {
          target: 'http://localhost:3000',
          changeOrigin: true,
          ws: true,
        },
      },
    },
  }
})
