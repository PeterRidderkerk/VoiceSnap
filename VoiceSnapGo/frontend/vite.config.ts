import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      // Not exported from package.json; needed for browser stub before full runtime init
      "@wailsio/runtime/runtime.js": path.resolve(
        __dirname,
        "node_modules/@wailsio/runtime/dist/runtime.js"
      ),
    },
  },
  plugins: [svelte()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
