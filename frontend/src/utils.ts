const pad = (value: number) => String(value).padStart(2, "0");

/** «17.11.2026» */
export function formatDate(value: string) {
  const date = new Date(value);
  return `${pad(date.getDate())}.${pad(date.getMonth() + 1)}.${date.getFullYear()}`;
}

/** «17.11.2026 20:57» */
export function formatDateTime(value: string) {
  const date = new Date(value);
  return `${formatDate(value)} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** «сегодня в 20:57» / «вчера в 20:57» / «15.11.2026» */
export function formatSeen(value: string) {
  const date = new Date(value);
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((startOfDay(new Date()) - startOfDay(date)) / 86_400_000);
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  if (days <= 0) return `сегодня в ${time}`;
  if (days === 1) return `вчера в ${time}`;
  return formatDate(value);
}

export function shortUuid(value: string) {
  return value.slice(0, 8);
}

export function keyTitle(key: { name: string; uuid: string }) {
  return key.name || shortUuid(key.uuid);
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
