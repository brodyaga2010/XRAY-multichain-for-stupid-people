import path from "node:path";
import { defineConfig } from "vitest/config";

// Часовой пояс фиксируем для всех воркеров: форматирование дат использует локальное время.
process.env.TZ = "Europe/Moscow";

export default defineConfig({
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") }
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"]
  }
});
