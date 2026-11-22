import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  formatBytes,
  formatDate,
  formatDateTime,
  formatDuration,
  formatSeen,
  formatTime,
  keyTitle,
  matchesSearch,
  shortUuid
} from "./utils";

// Даты строим из локальных компонентов: результат не зависит от часового пояса машины.
const local = (y: number, m: number, d: number, h = 0, min = 0) => new Date(y, m - 1, d, h, min).toISOString();

describe("formatBytes", () => {
  it.each<[number, string]>([
    [0, "0 КБ"],
    [-1, "0 КБ"],
    [NaN, "0 КБ"],
    [1, "1 Б"],
    [1023, "1023 Б"],
    [1024, "1,0 КБ"],
    [1536, "1,5 КБ"],
    [102_400, "100 КБ"],
    [1024 ** 2, "1,0 МБ"],
    [1024 ** 2 - 1, "1,0 МБ"],
    [1024 ** 3 - 1024, "1,0 ГБ"],
    [5 * 1024 ** 2 + 512 * 1024, "5,5 МБ"],
    [1024 ** 3, "1,0 ГБ"],
    [1024 ** 4, "1,0 ТБ"],
    [2048 * 1024 ** 4, "2048 ТБ"]
  ])("%d -> %s", (bytes, expected) => {
    expect(formatBytes(bytes)).toBe(expected);
  });
});

describe("formatDate / formatDateTime / formatTime", () => {
  it.each<[string, string, string, string]>([
    ["обычная дата", local(2026, 11, 17, 20, 57), "17.11.2026", "17.11.2026 20:57"],
    ["ведущие нули", local(2026, 1, 5, 3, 4), "05.01.2026", "05.01.2026 03:04"],
    ["полночь", local(2026, 12, 31, 0, 0), "31.12.2026", "31.12.2026 00:00"],
    ["конец суток", local(2026, 2, 28, 23, 59), "28.02.2026", "28.02.2026 23:59"]
  ])("%s", (_name, iso, date, dateTime) => {
    expect(formatDate(iso)).toBe(date);
    expect(formatDateTime(iso)).toBe(dateTime);
    expect(formatTime(iso)).toBe(dateTime.slice(-5));
  });

  it("formatTime принимает Date", () => {
    expect(formatTime(new Date(2026, 10, 17, 12, 31))).toBe("12:31");
  });

  it("нечитаемая дата даёт NaN в тексте, но не бросает", () => {
    expect(formatDate("не дата")).toContain("NaN");
  });
});

describe("formatSeen", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 10, 17, 21, 0));
  });
  afterEach(() => vi.useRealTimers());

  it.each<[string, string, string]>([
    ["сегодня", local(2026, 11, 17, 20, 57), "сегодня в 20:57"],
    ["сегодня, начало суток", local(2026, 11, 17, 0, 0), "сегодня в 00:00"],
    ["вчера поздно вечером", local(2026, 11, 16, 23, 59), "вчера в 23:59"],
    ["вчера рано утром", local(2026, 11, 16, 0, 1), "вчера в 00:01"],
    ["позавчера — только дата", local(2026, 11, 15, 12, 0), "15.11.2026"],
    ["давно", local(2025, 1, 2, 12, 0), "02.01.2025"],
    ["позже сегодня считается «сегодня»", local(2026, 11, 17, 23, 0), "сегодня в 23:00"],
    ["будущий день считается «сегодня»", local(2026, 11, 20, 10, 0), "сегодня в 10:00"]
  ])("%s", (_name, iso, expected) => {
    expect(formatSeen(iso)).toBe(expected);
  });
});

describe("formatDuration", () => {
  it.each<[number, string]>([
    [0, "0 с"],
    [-5000, "0 с"],
    [400, "0 с"],
    [500, "1 с"],
    [45_000, "45 с"],
    [59_499, "59 с"],
    [60_000, "1 мин"],
    [12 * 60_000, "12 мин"],
    [59 * 60_000 + 59_000, "59 мин"],
    [60 * 60_000, "1 ч"],
    [(2 * 60 + 5) * 60_000, "2 ч 5 мин"],
    [24 * 60 * 60_000, "24 ч"]
  ])("%d мс -> %s", (ms, expected) => {
    expect(formatDuration(ms)).toBe(expected);
  });
});

describe("shortUuid / keyTitle", () => {
  const uuid = "123e4567-e89b-12d3-a456-426614174000";

  it("shortUuid берёт первые 8 символов", () => {
    expect(shortUuid(uuid)).toBe("123e4567");
    expect(shortUuid("abc")).toBe("abc");
    expect(shortUuid("")).toBe("");
  });

  it.each<[string, string]>([
    ["Телефон Ани", "Телефон Ани"],
    ["", "123e4567"]
  ])("keyTitle(name=%j) -> %s", (name, expected) => {
    expect(keyTitle({ name, uuid })).toBe(expected);
  });
});

describe("matchesSearch", () => {
  it.each<[string, Array<string | number | null | undefined>, string, boolean]>([
    ["пустой запрос — всё подходит", ["a"], "", true],
    ["запрос из пробелов — всё подходит", ["a"], "   ", true],
    ["подстрока", ["Иван Петров"], "петр", true],
    ["регистр не важен", ["Иван"], "ИВАН", true],
    ["пробелы по краям запроса обрезаются", ["Иван"], "  иван ", true],
    ["любое из значений", ["x", "y", "нужный"], "нуж", true],
    ["число приводится к строке", [4242], "42", true],
    ["нет совпадений", ["Иван"], "Пётр", false],
    ["null и undefined не совпадают с текстом", [null, undefined], "null", false],
    ["пустой список значений", [], "a", false],
    ["пустой список и пустой запрос", [], "", true]
  ])("%s", (_name, values, query, expected) => {
    expect(matchesSearch(values, query)).toBe(expected);
  });
});

// copyText не тестируем: обёртка над navigator.clipboard, в node-окружении его нет.
