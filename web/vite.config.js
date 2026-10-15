import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const isolation = {
  "Cross-Origin-Opener-Policy": "same-origin",
  "Cross-Origin-Embedder-Policy": "require-corp",
};

export default defineConfig({
  plugins: [react()],
  server: {
    headers: isolation,
    // The Go server (cmd/server) answers /api; run it on its default port.
    proxy: { "/api": "http://127.0.0.1:8080" },
  },
  preview: {
    headers: isolation,
    proxy: { "/api": "http://127.0.0.1:8080" },
  },
});
