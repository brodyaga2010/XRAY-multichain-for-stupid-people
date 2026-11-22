import { describe, expect, it } from "vitest";
import type { AuditEntry } from "@/api";
import { auditQueryString, prependNew } from "./audit";

const entry = (id: number) => ({ id }) as AuditEntry;

describe("auditQueryString", () => {
  it.each([
    [{}, "limit=100"],
    [{ category: "keys" as const }, "limit=100&category=keys"],
    [{ category: "all" as const, q: "  " }, "limit=100"],
    [{ q: " anna@ ", before: 42, limit: 20 }, "limit=20&q=anna%40&before=42"]
  ])("%j → %s", (query, want) => {
    expect(auditQueryString(query)).toBe(want);
  });
});

describe("prependNew", () => {
  it("добавляет только новые записи сверху", () => {
    const current = [entry(3), entry(2), entry(1)];
    expect(prependNew(current, [entry(5), entry(4), entry(3)]).map((e) => e.id)).toEqual([5, 4, 3, 2, 1]);
  });

  it("без новых записей возвращает тот же массив", () => {
    const current = [entry(2), entry(1)];
    expect(prependNew(current, [entry(2), entry(1)])).toBe(current);
  });
});
