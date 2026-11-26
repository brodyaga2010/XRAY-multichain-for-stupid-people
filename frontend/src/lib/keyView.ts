/** Вид списка ключей: карточки сеткой или таблица. */
export type KeyView = "cards" | "table";

/** Сохранённое значение вида; неизвестное или пустое — fallback. */
export function parseKeyView(raw: string | null | undefined, fallback: KeyView): KeyView {
  return raw === "cards" || raw === "table" ? raw : fallback;
}

/** Вид, выбранный в этом браузере для списка storageKey (хранилище может быть недоступно — тогда fallback). */
export function loadKeyView(storageKey: string, fallback: KeyView): KeyView {
  try {
    return parseKeyView(localStorage.getItem(storageKey), fallback);
  } catch {
    return fallback;
  }
}

export function saveKeyView(storageKey: string, view: KeyView) {
  try {
    localStorage.setItem(storageKey, view);
  } catch {
    // приватный режим или запрет хранилища — вид просто не запомнится
  }
}
