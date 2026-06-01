import { VpnKey } from "./api";

export function formatDate(value: string) {
  return new Intl.DateTimeFormat("en", { month: "short", day: "numeric", year: "numeric" }).format(
    new Date(value)
  );
}

export function downloadJson(key: VpnKey) {
  const blob = new Blob([JSON.stringify(key.client_config_json, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${key.name}_xhttp_config.json`;
  link.click();
  URL.revokeObjectURL(url);
}

export function matchesSearch(values: Array<string | number | null | undefined>, query: string) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  return values.some((value) => String(value ?? "").toLowerCase().includes(normalized));
}
