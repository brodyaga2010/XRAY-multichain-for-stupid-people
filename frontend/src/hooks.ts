import { useEffect, useRef, useState } from "react";

export function useMediaQuery(query: string) {
  const getMatches = () => {
    if (typeof window === "undefined") return false;
    return window.matchMedia(query).matches;
  };

  const [matches, setMatches] = useState(getMatches);

  useEffect(() => {
    const media = window.matchMedia(query);
    const update = () => setMatches(media.matches);

    update();
    if (media.addEventListener) {
      media.addEventListener("change", update);
      return () => media.removeEventListener("change", update);
    }

    media.addListener(update);
    return () => media.removeListener(update);
  }, [query]);

  return matches;
}

/** Как часто открытая страница тихо перезапрашивает данные (трафик, онлайн, статусы). */
export const AUTO_REFRESH_MS = 15_000;

/**
 * Периодически вызывает refresh, пока вкладка видима, и сразу — при возврате на вкладку.
 * Ошибки фонового обновления не показываются: следующее обновление или действие пользователя их проявит.
 */
export function useAutoRefresh(refresh: () => Promise<unknown>, intervalMs = AUTO_REFRESH_MS) {
  const latest = useRef(refresh);
  latest.current = refresh;

  useEffect(() => {
    let running = false;
    const tick = () => {
      if (running || document.visibilityState !== "visible") return;
      running = true;
      latest
        .current()
        .catch(() => undefined)
        .finally(() => {
          running = false;
        });
    };

    const timer = window.setInterval(tick, intervalMs);
    document.addEventListener("visibilitychange", tick);
    window.addEventListener("focus", tick);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", tick);
      window.removeEventListener("focus", tick);
    };
  }, [intervalMs]);
}
