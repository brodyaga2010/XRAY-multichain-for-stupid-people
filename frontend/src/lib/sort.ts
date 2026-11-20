import { useMemo, useState } from "react";

export type SortDir = "asc" | "desc";
export type SortState<K extends string> = { key: K; dir: SortDir };

export type SortColumn<T> = {
  /** Значение для сравнения; null — «нет данных», такие строки всегда в конце. */
  value: (item: T) => string | number | null;
  /** Направление при первом нажатии: даты, трафик, счётчики — по убыванию. */
  firstDir?: SortDir;
};

const collator = new Intl.Collator("ru", { numeric: true, sensitivity: "base" });

/** Устойчивая сортировка: равные строки сохраняют исходный порядок. */
export function sortItems<T, K extends string>(items: T[], columns: Record<K, SortColumn<T>>, state: SortState<K>): T[] {
  const column = columns[state.key];
  const sign = state.dir === "asc" ? 1 : -1;
  return items
    .map((item, index) => ({ item, index, value: column.value(item) }))
    .sort((a, b) => {
      if (a.value === null || b.value === null) {
        if (a.value === b.value) return a.index - b.index;
        return a.value === null ? 1 : -1;
      }
      const diff =
        typeof a.value === "number" && typeof b.value === "number"
          ? a.value - b.value
          : collator.compare(String(a.value), String(b.value));
      return diff !== 0 ? diff * sign : a.index - b.index;
    })
    .map(({ item }) => item);
}

/** Сортировка по колонкам: повторное нажатие на колонку меняет направление. */
export function useSort<T, K extends string>(items: T[], columns: Record<K, SortColumn<T>>, initial: SortState<K>) {
  const [sort, setSort] = useState<SortState<K>>(initial);
  const sorted = useMemo(() => sortItems(items, columns, sort), [items, columns, sort]);

  function toggle(key: K) {
    setSort((current) =>
      current.key === key
        ? { key, dir: current.dir === "asc" ? "desc" : "asc" }
        : { key, dir: columns[key].firstDir ?? "asc" }
    );
  }

  return { sorted, sort, toggle };
}

/** Время в миллисекундах или null, если даты нет. */
export function timeOf(value: string | null | undefined): number | null {
  return value ? new Date(value).getTime() : null;
}
