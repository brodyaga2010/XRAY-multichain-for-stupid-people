import { useSyncExternalStore } from "react";

export type ThemeChoice = "light" | "dark" | "system";

const STORAGE_KEY = "theme";
const darkQuery = window.matchMedia("(prefers-color-scheme: dark)");

function readChoice(): ThemeChoice {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "light" || saved === "dark") return saved;
  } catch {
    // приватный режим или запрет хранилища — берём системную тему
  }
  return "system";
}

let current = readChoice();
const listeners = new Set<() => void>();

function apply() {
  const dark = current === "dark" || (current === "system" && darkQuery.matches);
  document.documentElement.classList.toggle("dark", dark);
}

function setTheme(choice: ThemeChoice) {
  current = choice;
  try {
    if (choice === "system") localStorage.removeItem(STORAGE_KEY);
    else localStorage.setItem(STORAGE_KEY, choice);
  } catch {
    // выбор просто не запомнится
  }
  apply();
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

apply();
darkQuery.addEventListener("change", apply);

/** Тема кабинета: светлая, тёмная или как в системе; одна на всю страницу, выбор запоминается в браузере. */
export function useTheme() {
  const choice = useSyncExternalStore(subscribe, () => current);
  return [choice, setTheme] as const;
}
