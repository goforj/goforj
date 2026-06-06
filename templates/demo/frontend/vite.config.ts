import path from 'node:path'
import { defineConfig, loadEnv } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { resolveGoForjFrontendEnv } from './goforj.env'

export default defineConfig(({ mode }) => {
  const appRoot = path.resolve(__dirname, '../../..')
  const appEnv = loadEnv(mode, appRoot, '')
  const frontendEnv = loadEnv(mode, process.cwd(), '')
  const env = { ...appEnv, ...frontendEnv }
  const goforjEnv = resolveGoForjFrontendEnv({ env, frontendDir: __dirname, projectRoot: appRoot })

  return {
    define: goforjEnv.define,
    plugins: [vue(), tailwindcss()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      proxy: {
        '/api': {
          target: goforjEnv.backendTarget,
          ws: true,
          rewriteWsOrigin: true,
          changeOrigin: true,
        },
        '/lighthouse': {
          target: goforjEnv.backendTarget,
          ws: true,
          changeOrigin: true,
        },
      },
    },
  }
})
