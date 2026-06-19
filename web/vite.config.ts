import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
// The dev server proxies /api/* to the Go backend so the frontend can call the
// API on the same origin during development (CSRF/cookies work without CORS).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: false,
      },
    },
    // On hosts where inotify is unavailable or limited (e.g. some containers /
    // sandboxes), set HOMEPAGE_DEV_POLL=1 to fall back to file polling for HMR.
    watch: process.env.HOMEPAGE_DEV_POLL
      ? { usePolling: true, interval: 500 }
      : undefined,
  },
  build: {
    outDir: "dist",
    sourcemap: true,
  },
});
