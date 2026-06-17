import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, loadEnv } from "vite"
import { resolveGoForjFrontendEnv } from "./goforj.env"

export default defineConfig(({ mode }) => {
  const projectRoot = path.resolve(__dirname, "../../..")
  const env = loadEnv(mode, projectRoot, "")
  const frontendEnv = resolveGoForjFrontendEnv({ env, frontendDir: __dirname, projectRoot })

  return {
    envDir: projectRoot,
    define: frontendEnv.define,
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      port: 5173,
      proxy: {
        "/api": {
          target: frontendEnv.backendTarget,
          changeOrigin: true,
          ws: true,
        },
      },
    },
  }
})
