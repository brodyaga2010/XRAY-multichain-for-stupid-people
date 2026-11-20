import { describe, expect, it } from "vitest";
import type { MonitorPoint } from "@/api";
import { DAY_MS, formatSeconds, latencyBars } from "./latency";

const now = Date.UTC(2026, 10, 20, 10, 38);
const end = Date.UTC(2026, 10, 20, 11, 0);
const at = (minutesBeforeEnd: number) => new Date(end - minutesBeforeEnd * 60_000).toISOString();
const ok = (minutesBeforeEnd: number, ms: number): MonitorPoint => ({ at: at(minutesBeforeEnd), ms, status: "ok" });
const fail = (minutesBeforeEnd: number): MonitorPoint => ({ at: at(minutesBeforeEnd), ms: null, status: "fail" });

describe("latencyBars", () => {
  it("сутки по получасам, выровнены по :00 и :30", () => {
    const { bars, from } = latencyBars([], now);
    expect(bars).toHaveLength(48);
    expect(from).toBe(end - DAY_MS);
    expect(bars[47].end).toBe(end);
    expect(bars.every((bar) => bar.height === 0 && bar.checks === 0)).toBe(true);
  });

  it.each([
    {
      name: "средняя и пик удачных проверок, шкала — круглая с запасом",
      points: [ok(10, 100), ok(20, 140)],
      want: { checks: 2, failed: 0, avg: 120, peak: 140, height: 60 },
      scale: 200,
      avg: 120
    },
    {
      name: "сбой — столбик во всю высоту, средняя по удачным",
      points: [ok(10, 100), fail(20)],
      want: { checks: 2, failed: 1, avg: 100, peak: 100, height: 100 },
      scale: 200,
      avg: 100
    },
    {
      name: "только сбои — без средней",
      points: [fail(10), fail(20)],
      want: { checks: 2, failed: 2, avg: null, peak: null, height: 100 },
      scale: 100,
      avg: null
    },
    {
      name: "большая задержка — шкала растёт",
      points: [ok(10, 1500)],
      want: { checks: 1, failed: 0, avg: 1500, peak: 1500, height: 75 },
      scale: 2000,
      avg: 1500
    }
  ])("$name", ({ points, want, scale, avg }) => {
    const result = latencyBars(points, now);
    const { start, end: barEnd, ...last } = result.bars[47];
    expect(last).toEqual(want);
    expect(barEnd - start).toBe(30 * 60_000);
    expect(result.scale).toBe(scale);
    expect(result.avg).toBe(avg);
  });

  it("маленькая задержка не пропадает, точки вне суток не учитываются", () => {
    const { bars } = latencyBars([ok(10, 300), ok(40, 1), ok(24 * 60 + 5, 999), ok(-5, 999)], now);
    expect(bars[46].height).toBe(3);
    expect(bars.reduce((sum, bar) => sum + bar.checks, 0)).toBe(2);
  });
});

describe("formatSeconds", () => {
  it.each([
    [118.7, "0,12 с"],
    [1500, "1,50 с"]
  ])("%s → %s", (ms, want) => {
    expect(formatSeconds(ms)).toBe(want);
  });
});
