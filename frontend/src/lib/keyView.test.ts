import { afterEach, describe, expect, it, vi } from "vitest";
import { KeyView, loadKeyView, parseKeyView, saveKeyView } from "./keyView";

describe("parseKeyView", () => {
  it.each<[string, string | null | undefined, KeyView, KeyView]>([
    ["таблица", "table", "cards", "table"],
    ["карточки", "cards", "table", "cards"],
    ["нет значения — по умолчанию", null, "table", "table"],
    ["undefined — по умолчанию", undefined, "cards", "cards"],
    ["мусор — по умолчанию", "grid", "cards", "cards"],
    ["регистр важен", "TABLE", "cards", "cards"]
  ])("%s", (_name, raw, fallback, want) => {
    expect(parseKeyView(raw, fallback)).toBe(want);
  });
});

describe("loadKeyView / saveKeyView", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("запоминает выбор по ключу списка", () => {
    const store = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => store.set(key, value)
    });
    expect(loadKeyView("keys-view:own", "cards")).toBe("cards");
    saveKeyView("keys-view:own", "table");
    expect(loadKeyView("keys-view:own", "cards")).toBe("table");
    expect(loadKeyView("keys-view:admin", "cards")).toBe("cards");
  });

  it("хранилище недоступно — вид по умолчанию, без ошибок", () => {
    const broken = () => {
      throw new Error("denied");
    };
    vi.stubGlobal("localStorage", { getItem: broken, setItem: broken });
    expect(() => saveKeyView("keys-view:own", "table")).not.toThrow();
    expect(loadKeyView("keys-view:own", "table")).toBe("table");
  });
});
