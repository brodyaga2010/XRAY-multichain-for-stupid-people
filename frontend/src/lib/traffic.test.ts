import { describe, expect, it } from "vitest";
import { formatDay, trafficBars } from "./traffic";

describe("trafficBars", () => {
  it("высота — доля от максимума, маленький день не пропадает", () => {
    const { bars, total, peak } = trafficBars([
      { day: "2026-11-18", up: 50, down: 150 },
      { day: "2026-11-19", up: 0, down: 0 },
      { day: "2026-11-20", up: 0, down: 1 }
    ]);
    expect(bars.map((bar) => bar.height)).toEqual([100, 0, 2]);
    expect(total).toBe(201);
    expect(peak).toBe(200);
  });

  it("пустой период — все нули", () => {
    const { bars, total } = trafficBars([{ day: "2026-11-20", up: 0, down: 0 }]);
    expect(bars[0].height).toBe(0);
    expect(total).toBe(0);
  });

  it("без дней", () => {
    expect(trafficBars([])).toEqual({ bars: [], total: 0, peak: 0 });
  });
});

describe("formatDay", () => {
  it.each([
    ["2026-11-20", "20 нояб"],
    ["2026-01-01", "1 янв"]
  ])("%s → %s", (day, want) => {
    expect(formatDay(day)).toBe(want);
  });
});
