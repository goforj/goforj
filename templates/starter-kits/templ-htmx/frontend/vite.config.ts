import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig } from "vite"

const isWatchBuild = process.argv.includes("--watch") || process.argv.includes("-w")

export default defineConfig({
  plugins: [tailwindcss()],
  build: {
    emptyOutDir: false,
    outDir: "dist",
    watch: isWatchBuild ? {
      exclude: ["dist/**", "node_modules/**"],
    } : null,
    rollupOptions: {
      input: {
        app: resolve(__dirname, "src/app.ts"),
      },
      onwarn(warning, defaultHandler) {
        if (warning.code === "EVAL" && warning.id?.includes("htmx.org")) {
          return
        }
        defaultHandler(warning)
      },
      output: {
        entryFileNames: "app.js",
        assetFileNames: "app.[ext]",
      },
    },
  },
})
