import { describe, expect, it } from "vitest";
import { defaultView, pathForView, viewFromPath } from "./routes";
import type { AppView } from "./types";

describe("defaultView", () => {
  it.each([
    [true, "dashboard"],
    [false, "keys"]
  ] as const)("isAdmin=%s -> %s", (isAdmin, expected) => {
    expect(defaultView(isAdmin)).toBe(expected);
  });
});

describe("viewFromPath", () => {
  it.each<[string, string, boolean, AppView]>([
    ["админ: корень", "/", true, "dashboard"],
    ["админ: статус", "/status", true, "status"],
    ["админ: пользователи", "/users", true, "admin"],
    ["админ: ключи", "/keys", true, "keys"],
    ["админ: журнал", "/audit", true, "audit"],
    ["админ: профиль", "/profile", true, "profile"],
    ["пользователь: ключи", "/keys", false, "keys"],
    ["пользователь: профиль", "/profile", false, "profile"],
    ["пользователь: корень", "/", false, "keys"],
    ["завершающий слэш", "/users/", true, "admin"],
    ["несколько слэшей в конце", "/audit///", true, "audit"],
    ["пустая строка = корень", "", true, "dashboard"],
    ["неизвестный путь, админ", "/nope", true, "dashboard"],
    ["неизвестный путь, пользователь", "/nope", false, "keys"],
    ["вложенный путь не совпадает", "/users/42", true, "dashboard"],
    ["регистр важен", "/Users", true, "dashboard"],
    ["раздел админа для пользователя", "/users", false, "keys"],
    ["статус для пользователя", "/status", false, "keys"],
    ["журнал для пользователя", "/audit", false, "keys"]
  ])("%s", (_name, path, isAdmin, expected) => {
    expect(viewFromPath(path, isAdmin)).toBe(expected);
  });
});

describe("pathForView", () => {
  it.each<[AppView, boolean, string]>([
    ["dashboard", true, "/"],
    ["status", true, "/status"],
    ["admin", true, "/users"],
    ["keys", true, "/keys"],
    ["audit", true, "/audit"],
    ["profile", true, "/profile"],
    ["keys", false, "/"],
    ["profile", false, "/profile"]
  ])("%s, isAdmin=%s -> %s", (view, isAdmin, expected) => {
    expect(pathForView(view, isAdmin)).toBe(expected);
  });

  it("путь раздела обратно даёт тот же раздел", () => {
    const adminViews: AppView[] = ["dashboard", "status", "admin", "keys", "audit", "profile"];
    for (const view of adminViews) {
      expect(viewFromPath(pathForView(view, true), true)).toBe(view);
    }
    const userViews: AppView[] = ["keys", "profile"];
    for (const view of userViews) {
      expect(viewFromPath(pathForView(view, false), false)).toBe(view);
    }
  });
});
