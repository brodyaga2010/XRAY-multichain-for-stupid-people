import { useEffect, useMemo, useState } from "react";
import { api, AuditEntry, User, VpnKey } from "@/api";
import { useAutoRefresh } from "@/hooks";
import { SortableHead } from "@/components/SortableHead";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableHeader, TableCell, TableRow } from "@/components/ui/table";
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
  "user.password_change": "Смена пароля"
};

type AuditSort = "date" | "actor" | "action" | "target";

const actionLabel = (entry: AuditEntry) => actionLabels[entry.action] ?? entry.action;

export function AuditLog({ setError }: { setError: (error: string) => void }) {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [keys, setKeys] = useState<VpnKey[]>([]);
  const [users, setUsers] = useState<User[]>([]);

  useEffect(() => {
    api
      .adminAudit(200)
      .then(setEntries)
      .catch((err) => setError(err.message));
    // Справочники нужны только для подписей: если не загрузились — показываем «тип #id».
    api.adminKeys().then(setKeys).catch(() => undefined);
    api.adminUsers().then(setUsers).catch(() => undefined);
  }, []);

  useAutoRefresh(() => api.adminAudit(200).then(setEntries));

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
    if (entry.target_type === "user") {
      const user = userById.get(id);
      if (user) return user.email;
    }
    return `${entry.target_type} #${id}`;
  }

  return (
    <Card className="gap-4">
      <CardContent className="@container grid gap-3">
        {entries.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">Записей нет</p>
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
            <p className="text-xs text-muted-foreground">Последние записи: {entries.length}</p>
          </>
        )}
      </CardContent>
    </Card>
  );
}
