import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { ui: fileURLToPath(new URL("../ui/src/index.ts", import.meta.url)) },
  },
  server: {
    host: true,
    port: 5173,
    allowedHosts: ["admin.sms.localhost", "client.sms.localhost"],
    proxy: {
      "/admin/v1": { target: "http://127.0.0.1:8080", changeOrigin: false },
      "/healthz": { target: "http://127.0.0.1:8080", changeOrigin: false },
      "/readyz": { target: "http://127.0.0.1:8080", changeOrigin: false },
    },
  },
});
