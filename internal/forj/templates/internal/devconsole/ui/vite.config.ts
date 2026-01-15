import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  base: "/__devconsole/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
