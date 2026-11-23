import { describe, expect, it } from "vitest";
import { keyPreview } from "./keyLink";

describe("keyPreview", () => {
  it.each([
    [
      "ссылка VLESS — начало UUID и адрес",
      "vless://9755086b-86a2-4aa5-8675-6095607a556b@kemsekov.ru:443?type=tcp&security=reality#%D0%AF%D0%BD",
      "vless://9755086b…@kemsekov.ru:443"
    ],
    ["без порта и параметров", "vless://9755086b-86a2-4aa5@example.org", "vless://9755086b…@example.org"],
    ["короткий идентификатор не режется", "trojan://abc@example.org:8443/path", "trojan://abc@example.org:8443"],
    ["пробелы по краям", "  vless://12345678abcd@h:1  ", "vless://12345678…@h:1"],
    ["не ссылка — как есть", "просто текст", "просто текст"],
    ["без идентификатора — как есть", "https://example.org/path", "https://example.org/path"],
    ["пустая строка", "", ""]
  ])("%s", (_name, uri, want) => {
    expect(keyPreview(uri)).toBe(want);
  });
});
