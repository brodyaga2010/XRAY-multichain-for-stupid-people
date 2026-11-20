import { describe, expect, it } from "vitest";
import { sortItems, timeOf, type SortColumn } from "./sort";

type Row = { id: string; n: number | null; s: string | null };

const columns: Record<"n" | "s", SortColumn<Row>> = {
  n: { value: (r) => r.n },
  s: { value: (r) => r.s }
};

const ids = (rows: Row[]) => rows.map((r) => r.id);
const row = (id: string, n: number | null, s: string | null = null): Row => ({ id, n, s });

describe("sortItems: числа", () => {
  const rows = [row("a", 3), row("b", 1), row("c", 2)];

  it.each([
    ["asc", ["b", "c", "a"]],
    ["desc", ["a", "c", "b"]]
  ] as const)("направление %s", (dir, expected) => {
    expect(ids(sortItems(rows, columns, { key: "n", dir }))).toEqual(expected);
  });

  it("не мутирует исходный массив", () => {
    const copy = [...rows];
    sortItems(rows, columns, { key: "n", dir: "asc" });
    expect(rows).toEqual(copy);
  });

  it("пустой и одноэлементный списки", () => {
    expect(sortItems([], columns, { key: "n", dir: "asc" })).toEqual([]);
    expect(ids(sortItems([row("x", 1)], columns, { key: "n", dir: "desc" }))).toEqual(["x"]);
  });

  it("отрицательные числа и ноль", () => {
    const items = [row("a", 0), row("b", -5), row("c", 5)];
    expect(ids(sortItems(items, columns, { key: "n", dir: "asc" }))).toEqual(["b", "a", "c"]);
  });
});

describe("sortItems: null всегда в конце", () => {
  const rows = [row("a", null), row("b", 2), row("c", null), row("d", 1)];

  it.each([
    ["asc", ["d", "b", "a", "c"]],
    ["desc", ["b", "d", "a", "c"]]
  ] as const)("направление %s, null сохраняют исходный порядок", (dir, expected) => {
    expect(ids(sortItems(rows, columns, { key: "n", dir }))).toEqual(expected);
  });

  it("все значения null — порядок не меняется", () => {
    const items = [row("a", null), row("b", null), row("c", null)];
    expect(ids(sortItems(items, columns, { key: "n", dir: "desc" }))).toEqual(["a", "b", "c"]);
  });

  it("ноль — не null", () => {
    const items = [row("a", null), row("b", 0)];
    expect(ids(sortItems(items, columns, { key: "n", dir: "asc" }))).toEqual(["b", "a"]);
  });
});

describe("sortItems: устойчивость", () => {
  const rows = [row("a", 1), row("b", 1), row("c", 0), row("d", 1)];

  it.each([
    ["asc", ["c", "a", "b", "d"]],
    ["desc", ["a", "b", "d", "c"]]
  ] as const)("равные значения сохраняют порядок (%s)", (dir, expected) => {
    expect(ids(sortItems(rows, columns, { key: "n", dir }))).toEqual(expected);
  });

  it("строки, равные без учёта регистра, сохраняют порядок", () => {
    const items = [row("1", 0, "Борис"), row("2", 0, "борис"), row("3", 0, "БОРИС")];
    expect(ids(sortItems(items, columns, { key: "s", dir: "desc" }))).toEqual(["1", "2", "3"]);
  });
});

describe("sortItems: строки (русская сортировка)", () => {
  const sorted = (values: string[], dir: "asc" | "desc") =>
    sortItems(
      values.map((s, i) => row(String(i), 0, s)),
      columns,
      { key: "s", dir }
    ).map((r) => r.s);

  it.each([
    ["русские буквы по алфавиту", ["Яна", "Анна", "Борис"], ["Анна", "Борис", "Яна"]],
    ["цифры сравниваются как числа", ["ключ 10", "ключ 2", "ключ 1"], ["ключ 1", "ключ 2", "ключ 10"]],
    ["регистр не важен", ["борис", "Анна", "вера"], ["Анна", "борис", "вера"]],
    ["ё рядом с е", ["Жанна", "Ёлка", "Дима"], ["Дима", "Ёлка", "Жанна"]]
  ])("asc: %s", (_name, input, expected) => {
    expect(sorted(input, "asc")).toEqual(expected);
  });

  it("desc — обратный порядок", () => {
    expect(sorted(["ключ 10", "ключ 2", "ключ 1"], "desc")).toEqual(["ключ 10", "ключ 2", "ключ 1"]);
  });

  it("null среди строк — в конце в обоих направлениях", () => {
    const items = [row("a", 0, null), row("b", 0, "Б"), row("c", 0, "А")];
    expect(ids(sortItems(items, columns, { key: "s", dir: "asc" }))).toEqual(["c", "b", "a"]);
    expect(ids(sortItems(items, columns, { key: "s", dir: "desc" }))).toEqual(["b", "c", "a"]);
  });

  it("число и строка в одной колонке сравниваются как строки", () => {
    const mixed: Record<"v", SortColumn<{ v: string | number }>> = { v: { value: (r) => r.v } };
    const out = sortItems([{ v: "10" }, { v: 9 }], mixed, { key: "v", dir: "asc" });
    expect(out.map((r) => r.v)).toEqual([9, "10"]);
  });
});

describe("timeOf", () => {
  it.each([
    ["ISO-строка с Z", "2026-11-17T20:57:00Z", Date.UTC(2026, 10, 17, 20, 57)],
    ["ISO-строка со смещением", "2026-11-17T23:57:00+03:00", Date.UTC(2026, 10, 17, 20, 57)],
    ["эпоха", "1970-01-01T00:00:00Z", 0]
  ])("%s", (_name, input, expected) => {
    expect(timeOf(input)).toBe(expected);
  });

  it.each([[null], [undefined], [""]])("пустое значение %j — null", (input) => {
    expect(timeOf(input)).toBeNull();
  });

  it("нечитаемая дата даёт NaN", () => {
    expect(timeOf("не дата")).toBeNaN();
  });
});
