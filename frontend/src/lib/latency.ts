import type { MonitorPoint } from "@/api";

export const DAY_MS = 24 * 60 * 60 * 1000;
const BUCKET_MS = 30 * 60 * 1000;
/** «Круглые» верхние границы шкалы, мс: столбики не упираются в потолок и не прижимаются к полу. */
const SCALE_STEPS = [100, 200, 300, 500, 1000, 2000, 3000, 5000, 10000];

export interface LatencyBar {
  start: number;
  end: number;
  /** Проверок в интервале. */
  checks: number;
  /** Проверок без ответа. */
  failed: number;
  /** Средняя и наибольшая задержка удачных проверок, мс; null — удачных не было. */
  avg: number | null;
  peak: number | null;
  /** Высота столбика, 0..100 % от шкалы; со сбоем — во всю высоту. */
  height: number;
}

/**
 * Задержка за сутки по получасам: столбик — средняя задержка удачных проверок.
 * Интервалы выровнены по :00 и :30, последний заканчивается после now.
 */
export function latencyBars(points: MonitorPoint[], now: number, minHeight = 3) {
  const end = Math.ceil(now / BUCKET_MS) * BUCKET_MS;
  const from = end - DAY_MS;
  const count = DAY_MS / BUCKET_MS;
  const sums = Array.from({ length: count }, () => ({ checks: 0, failed: 0, ok: 0, sum: 0, peak: 0 }));
  let okCount = 0;
  let okSum = 0;
  for (const point of points) {
    const t = new Date(point.at).getTime();
    if (!(t >= from && t < end)) continue;
    const bucket = sums[Math.floor((t - from) / BUCKET_MS)];
    bucket.checks++;
    if (point.status === "fail" || point.ms == null) {
      bucket.failed++;
      continue;
    }
    bucket.ok++;
    bucket.sum += point.ms;
    bucket.peak = Math.max(bucket.peak, point.ms);
    okCount++;
    okSum += point.ms;
  }
  const avgs = sums.map((s) => (s.ok ? s.sum / s.ok : null));
  const top = Math.max(0, ...avgs.map((avg) => avg ?? 0));
  const scale = SCALE_STEPS.find((step) => step >= top * 1.15) ?? Math.ceil((top * 1.15) / 1000) * 1000;
  const bars: LatencyBar[] = sums.map((s, index) => {
    const avg = avgs[index];
    const height = s.failed ? 100 : avg == null ? 0 : Math.max(minHeight, Math.min(100, (avg / scale) * 100));
    return {
      start: from + index * BUCKET_MS,
      end: from + (index + 1) * BUCKET_MS,
      checks: s.checks,
      failed: s.failed,
      avg,
      peak: s.ok ? s.peak : null,
      height
    };
  });
  return { bars, from, end, scale, avg: okCount ? okSum / okCount : null };
}

/** «0,12 с» из миллисекунд. */
export function formatSeconds(ms: number) {
  return `${(ms / 1000).toFixed(2).replace(".", ",")} с`;
}
