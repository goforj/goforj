import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  base: "/__devconsole/",
  server: {
    port: 5173,
    proxy: {
      "/__devconsole/api": {
        target: "http://localhost:3000",
        ws: true,
      },
      "/__devconsole/ws": {
        target: "http://localhost:3000",
        ws: true,
      },
      "/__devconsole/auth": {
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
