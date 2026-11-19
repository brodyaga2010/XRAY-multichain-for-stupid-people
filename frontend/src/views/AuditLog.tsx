import { useEffect, useMemo, useState } from "react";
import { api, AuditEntry, User, VpnKey } from "@/api";
import { useAutoRefresh } from "@/hooks";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
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
      <CardHeader>
        <CardTitle>Журнал действий</CardTitle>
        <CardDescription>записей: {entries.length}</CardDescription>
      </CardHeader>
      <CardContent className="@container">
        {entries.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">Записей нет</p>
        ) : (
          <>
            <div className="hidden overflow-hidden rounded-lg border @3xl:block">
              <Table>
                <TableHeader className="bg-muted/50">
                  <TableRow>
                    <TableHead className="pl-4">Дата</TableHead>
                    <TableHead>Кто</TableHead>
                    <TableHead>Действие</TableHead>
                    <TableHead>Объект</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((entry) => (
                    <TableRow key={entry.id}>
                      <TableCell className="pl-4 text-muted-foreground tabular-nums">{formatDateTime(entry.created_at)}</TableCell>
                      <TableCell className="max-w-56">
                        <span className="block truncate" title={entry.actor_email ?? undefined}>
                          {entry.actor_email ?? "—"}
                        </span>
                      </TableCell>
                      <TableCell className="font-medium">{actionLabels[entry.action] ?? entry.action}</TableCell>
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
              {entries.map((entry) => (
                <li key={entry.id} className="grid gap-0.5 py-2.5 first:pt-0 last:pb-0">
                  <div className="flex justify-between gap-3 text-xs text-muted-foreground">
                    <span className="tabular-nums">{formatDateTime(entry.created_at)}</span>
                    <span className="truncate" title={entry.actor_email ?? undefined}>
                      {entry.actor_email ?? "—"}
                    </span>
                  </div>
                  <span className="text-sm font-medium">{actionLabels[entry.action] ?? entry.action}</span>
                  <span className="truncate text-sm text-muted-foreground" title={target(entry)}>
                    {target(entry)}
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
      </CardContent>
    </Card>
  );
}
