import path from "path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { VitePWA } from "vite-plugin-pwa";

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    // Keeps the app itself on the device so it opens offline. A new version
    // waits until the person reloads (see UpdatePrompt), so a deploy never
    // reloads the page out from under them.
    VitePWA({
      registerType: "prompt",
      injectRegister: null,
      // public/manifest.json is linked from index.html already.
      manifest: false,
      workbox: {
        globPatterns: ["**/*.{js,css,html,svg,png,ico,json}"],
        navigateFallback: "/index.html",
        navigateFallbackDenylist: [/^\/api\//],
        cleanupOutdatedCaches: true,
      },
    }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    allowedHosts: [".ts.net"],
    proxy: {
      "/api": {
        target: process.env.API_URL ?? "http://localhost:3000",
        ws: true,
      },
    },
  },
});
