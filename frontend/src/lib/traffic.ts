import type { TrafficDay } from "@/api";

export type TrafficBar = TrafficDay & { total: number; /** высота столбика, 0..100 % от максимума */ height: number };

/** Столбики графика: высота — доля от самого «тяжёлого» дня; ненулевой день не тоньше minHeight %. */
export function trafficBars(days: TrafficDay[], minHeight = 2): { bars: TrafficBar[]; total: number; peak: number } {
  const totals = days.map((day) => day.up + day.down);
  const peak = totals.length ? Math.max(...totals) : 0;
  const bars = days.map((day, index) => {
    const total = totals[index];
    const height = peak === 0 || total === 0 ? 0 : Math.max(minHeight, (total / peak) * 100);
    return { ...day, total, height };
  });
  return { bars, total: totals.reduce((sum, value) => sum + value, 0), peak };
}

/** «20 ноя» из "2026-11-20" — без сдвига часового пояса (день уже посчитан сервером). */
export function formatDay(day: string) {
  const [year, month, date] = day.split("-").map(Number);
  return new Date(year, month - 1, date).toLocaleDateString("ru-RU", { day: "numeric", month: "short" }).replace(".", "");
}
