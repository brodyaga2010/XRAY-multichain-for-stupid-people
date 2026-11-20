import type { AuditEntry } from "@/api";

export type AuditCategory = "all" | "users" | "keys" | "account";

/** Сколько записей журнала грузить за раз. */
export const AUDIT_PAGE = 100;

export type AuditQuery = { category?: AuditCategory; q?: string; before?: number; limit?: number };

/** Строка запроса к /admin/audit; пустые параметры не передаются. */
export function auditQueryString({ category = "all", q = "", before, limit = AUDIT_PAGE }: AuditQuery) {
  const params = new URLSearchParams({ limit: String(limit) });
  if (category !== "all") params.set("category", category);
  if (q.trim()) params.set("q", q.trim());
  if (before !== undefined) params.set("before", String(before));
  return params.toString();
}

/** Добавляет сверху записи из свежей первой страницы, которых ещё нет в списке (фоновое обновление). */
export function prependNew(current: AuditEntry[], fresh: AuditEntry[]) {
  const known = new Set(current.map((entry) => entry.id));
  const added = fresh.filter((entry) => !known.has(entry.id));
  return added.length === 0 ? current : [...added, ...current];
}
