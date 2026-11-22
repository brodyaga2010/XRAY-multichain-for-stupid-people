import { useEffect, useMemo, useRef, useState } from "react";
import { api, AuditEntry, User, VpnKey } from "@/api";
import { useAutoRefresh } from "@/hooks";
import { FilterChips } from "@/components/FilterChips";
import { SearchBox } from "@/components/SearchBox";
import { SortableHead } from "@/components/SortableHead";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableHeader, TableCell, TableRow } from "@/components/ui/table";
import { AUDIT_PAGE, AuditCategory, prependNew } from "@/lib/audit";
import { SortColumn, timeOf, useSort } from "@/lib/sort";
import { formatDateTime, keyTitle } from "@/utils";

const actionLabels: Record<string, string> = {
  "key.create": "Создан ключ",
  "key.revoke": "Ключ отозван",
  "admin.key.create": "Админ создал ключ",
  "admin.key.revoke": "Админ отозвал ключ",
  "admin.key.import": "Импорт ключей",
  "user.approve": "Пользователь подтверждён",
  "user.suspend": "Пользователь заблокирован",
  "user.restore": "Пользователь восстановлен",
  "user.limit_update": "Изменён лимит",
  "user.password_change": "Смена пароля",
  "auth.password_reset": "Пароль сброшен по почте",
  "auth.email_verify": "Почта подтверждена",
  "auth.email_change": "Почта изменена",
  "subscription.assign": "Назначен тариф",
  "subscription.update": "Изменена подписка",
  "subscription.extend": "Подписка продлена",
  "subscription.expire": "Подписка закончилась",
  "plan.create": "Создан тариф",
  "plan.update": "Изменён тариф",
  "plan.delete": "Удалён тариф"
};

type AuditSort = "date" | "actor" | "action" | "target";

const actionLabel = (entry: AuditEntry) => actionLabels[entry.action] ?? entry.action;

const categories: Array<{ id: AuditCategory; label: string }> = [
  { id: "all", label: "Все" },
  { id: "users", label: "Пользователи" },
  { id: "keys", label: "Ключи" },
  { id: "account", label: "Аккаунт" }
];

export function AuditLog({ setError }: { setError: (error: string) => void }) {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [keys, setKeys] = useState<VpnKey[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [category, setCategory] = useState<AuditCategory>("all");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState(""); // query с задержкой: запрос к серверу — когда перестали печатать
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  // Ответ на устаревший фильтр не должен затереть свежий список.
  const generation = useRef(0);

  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(query), 300);
    return () => window.clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    const current = ++generation.current;
    setLoading(true);
    api
      .adminAudit({ category, q: search })
      .then((page) => {
        if (current !== generation.current) return;
        setEntries(page);
        setHasMore(page.length === AUDIT_PAGE);
      })
      .catch((err) => setError(err.message))
      .finally(() => current === generation.current && setLoading(false));
  }, [category, search]);

  useEffect(() => {
    // Справочники нужны только для подписей: если не загрузились — показываем «тип #id».
    api.adminKeys().then(setKeys).catch(() => undefined);
    api.adminUsers().then(setUsers).catch(() => undefined);
  }, []);

  useAutoRefresh(async () => {
    const current = generation.current;
    const page = await api.adminAudit({ category, q: search });
    if (current === generation.current) setEntries((list) => prependNew(list, page));
  });

  async function loadMore() {
    const last = entries[entries.length - 1];
    if (!last) return;
    const current = generation.current;
    setLoadingMore(true);
    try {
      const page = await api.adminAudit({ category, q: search, before: last.id });
      if (current !== generation.current) return;
      setEntries((list) => [...list, ...page.filter((entry) => !list.some((item) => item.id === entry.id))]);
      setHasMore(page.length === AUDIT_PAGE);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось загрузить записи");
    } finally {
      setLoadingMore(false);
    }
  }

  const keyById = useMemo(() => new Map(keys.map((key) => [String(key.id), key])), [keys]);
  const userById = useMemo(() => new Map(users.map((user) => [String(user.id), user])), [users]);

  const columns = useMemo<Record<AuditSort, SortColumn<AuditEntry>>>(
    () => ({
      date: { value: (entry) => timeOf(entry.created_at), firstDir: "desc" },
      actor: { value: (entry) => entry.actor_email },
      action: { value: actionLabel },
      target: { value: (entry) => (entry.target_type ? target(entry) : null) }
    }),
    [keyById, userById]
  );
  const { sorted, sort, toggle } = useSort<AuditEntry, AuditSort>(entries, columns, { key: "date", dir: "desc" });

  function target(entry: AuditEntry) {
    if (!entry.target_type) return "—";
    if (entry.target_id === null || entry.target_id === undefined) return entry.target_type;
    const id = String(entry.target_id);
    if (entry.target_type === "vpn_key") {
      const key = keyById.get(id);
      if (key) return `Ключ «${keyTitle(key)}»`;
    }
    if (entry.target_type === "plan") {
      const name = entry.audit_metadata?.name;
      return typeof name === "string" ? `Тариф «${name}»` : `Тариф #${id}`;
    }
    if (entry.target_type === "user") {
      const user = userById.get(id);
      if (user) return user.email;
    }
    return `${entry.target_type} #${id}`;
  }

  return (
    <Card className="gap-4">
      <CardContent className="@container grid gap-3">
        <div className="flex min-w-0 flex-col-reverse gap-3 md:flex-row md:items-center md:justify-between">
          <FilterChips label="Тип действия" options={categories} value={category} onChange={setCategory} className="md:shrink-0" />
          <SearchBox value={query} onChange={setQuery} placeholder="Почта, ключ" className="md:w-72" />
        </div>
        {entries.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">
            {loading ? "Загрузка…" : search || category !== "all" ? "Ничего не найдено" : "Записей нет"}
          </p>
        ) : (
          <>
            <div className="hidden overflow-hidden rounded-lg border @3xl:block">
              <Table>
                <TableHeader className="bg-muted/50">
                  <TableRow>
                    <SortableHead column="date" sort={sort} onSort={toggle} className="pl-4">
                      Дата
                    </SortableHead>
                    <SortableHead column="actor" sort={sort} onSort={toggle}>
                      Кто
                    </SortableHead>
                    <SortableHead column="action" sort={sort} onSort={toggle}>
                      Действие
                    </SortableHead>
                    <SortableHead column="target" sort={sort} onSort={toggle}>
                      Объект
                    </SortableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sorted.map((entry) => (
                    <TableRow key={entry.id}>
                      <TableCell className="pl-4 text-muted-foreground tabular-nums">{formatDateTime(entry.created_at)}</TableCell>
                      <TableCell className="max-w-56">
                        <span className="block truncate" title={entry.actor_email ?? undefined}>
                          {entry.actor_email ?? "—"}
                        </span>
                      </TableCell>
                      <TableCell className="font-medium">{actionLabel(entry)}</TableCell>
                      <TableCell className="max-w-72">
                        <span className="block truncate" title={target(entry)}>
                          {target(entry)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <ul className="grid divide-y @3xl:hidden">
              {sorted.map((entry) => (
                <li key={entry.id} className="grid gap-0.5 py-2.5 first:pt-0 last:pb-0">
                  <div className="flex justify-between gap-3 text-xs text-muted-foreground">
                    <span className="tabular-nums">{formatDateTime(entry.created_at)}</span>
                    <span className="truncate" title={entry.actor_email ?? undefined}>
                      {entry.actor_email ?? "—"}
                    </span>
                  </div>
                  <span className="text-sm font-medium">{actionLabel(entry)}</span>
                  <span className="truncate text-sm text-muted-foreground" title={target(entry)}>
                    {target(entry)}
                  </span>
                </li>
              ))}
            </ul>
            <div className="flex items-center justify-between gap-3">
              <p className="text-xs text-muted-foreground">
                {hasMore ? `Показаны последние ${entries.length}` : `Всего записей: ${entries.length}`}
              </p>
              {hasMore && (
                <Button variant="outline" disabled={loadingMore} onClick={loadMore}>
                  {loadingMore ? "Загрузка…" : "Загрузить ещё"}
                </Button>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
