const dateFormat = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short", year: "numeric" });
const dateTimeFormat = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit"
});

export function formatDate(value: string) {
  return dateFormat.format(new Date(value));
}

export function formatDateTime(value: string) {
  return dateTimeFormat.format(new Date(value));
}

export function formatBytes(bytes: number) {
  if (!bytes || bytes < 0) return "0 КБ";
  const units = ["Б", "КБ", "МБ", "ГБ", "ТБ"];
  if (bytes < 1024) return `${bytes} Б`;
  let value = bytes / 1024;
  let unit = 1;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 100 ? 0 : 1).replace(".", ",")} ${units[unit]}`;
}

export function matchesSearch(values: Array<string | number | null | undefined>, query: string) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  return values.some((value) => String(value ?? "").toLowerCase().includes(normalized));
}

export async function copyText(text: string) {
  await navigator.clipboard.writeText(text);
}
