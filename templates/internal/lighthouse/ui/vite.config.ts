import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  base: "/__lighthouse/",
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/__lighthouse/api": {
        target: "http://localhost:3000",
        ws: true,
      },
      "/__lighthouse/ws": {
        target: "http://localhost:3000",
        ws: true,
      },
      "/__lighthouse/auth": {
        target: "http://localhost:3000",
        ws: false,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
