import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// В разработке API отдаёт Go-сервер: cd server && XRAY_MODE=dry_run go run ./cmd/xray-panel
const proxyTarget = process.env.VITE_PROXY_TARGET ?? "http://127.0.0.1:8000";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: proxyTarget, changeOrigin: true },
      "/sub": { target: proxyTarget, changeOrigin: true }
    }
  }
});
