import { describe, expect, it } from "vitest";
import { daysWord, paidUntilOrder, planDuration, subscriptionState, toDateInput } from "./subscription";

const now = new Date("2026-11-20T09:00:00Z").getTime();

describe("subscriptionState", () => {
  it.each([
    ["бессрочно", { paid_until: null, subscription_active: true }, { kind: "forever" }],
    ["действует", { paid_until: "2026-12-20T18:59:59Z", subscription_active: true }, { kind: "active", until: "2026-12-20T18:59:59Z", daysLeft: 31 }],
    ["скоро конец", { paid_until: "2026-11-22T18:59:59Z", subscription_active: true }, { kind: "soon", until: "2026-11-22T18:59:59Z", daysLeft: 3 }],
    ["закончилась", { paid_until: "2026-11-19T18:59:59Z", subscription_active: false }, { kind: "expired", until: "2026-11-19T18:59:59Z" }],
    ["сервер говорит — закончилась", { paid_until: "2026-11-21T00:00:00Z", subscription_active: false }, { kind: "expired", until: "2026-11-21T00:00:00Z" }]
  ])("%s", (_, user, want) => {
    expect(subscriptionState(user, now)).toEqual(want);
  });
});

describe("paidUntilOrder", () => {
  it("бессрочные — в конце", () => {
    expect(paidUntilOrder({ paid_until: null })).toBeGreaterThan(paidUntilOrder({ paid_until: "2099-01-01T00:00:00Z" }));
  });
});

describe("toDateInput", () => {
  it.each([
    [null, ""],
    ["2026-12-20T18:59:59Z", "2026-12-20"], // 21:59 по Москве
    ["2026-12-20T21:30:00Z", "2026-12-21"] // 00:30 по Москве — уже следующий день
  ])("%s → %s", (iso, want) => {
    expect(toDateInput(iso)).toBe(want);
  });
});

describe("daysWord", () => {
  it.each([
    [1, "1 день"],
    [2, "2 дня"],
    [5, "5 дней"],
    [11, "11 дней"],
    [21, "21 день"],
    [24, "24 дня"],
    [112, "112 дней"]
  ])("%i → %s", (n, want) => {
    expect(daysWord(n)).toBe(want);
  });
});

describe("planDuration", () => {
  it("0 — бессрочно", () => {
    expect(planDuration(0)).toBe("бессрочно");
    expect(planDuration(30)).toBe("30 дней");
  });
});
