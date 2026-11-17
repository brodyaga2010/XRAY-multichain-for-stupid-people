import { useEffect, useMemo, useState } from "react";
import { api, AuditEntry, User, VpnKey } from "../api";
import { formatDateTime, keyTitle } from "../utils";

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
    <div className="table-panel audit-log animate-in">
      <div className="panel-heading">
        <h2>Журнал</h2>
        <span>записей: {entries.length}</span>
      </div>
      {entries.length === 0 ? (
        <div className="empty">Записей нет</div>
      ) : (
        <>
          <div className="responsive-table">
            <table>
              <thead>
                <tr>
                  <th>Дата</th>
                  <th>Кто</th>
                  <th>Действие</th>
                  <th>Объект</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.id}>
                    <td className="nowrap">{formatDateTime(entry.created_at)}</td>
                    <td>
                      <span className="ellipsis audit-cell" title={entry.actor_email ?? undefined}>
                        {entry.actor_email ?? "—"}
                      </span>
                    </td>
                    <td className="nowrap">{actionLabels[entry.action] ?? entry.action}</td>
                    <td>
                      <span className="ellipsis audit-cell" title={target(entry)}>
                        {target(entry)}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="audit-cards">
            {entries.map((entry) => (
              <article className="audit-card" key={entry.id}>
                <div className="audit-card-top">
                  <span>{formatDateTime(entry.created_at)}</span>
                  <span className="ellipsis" title={entry.actor_email ?? undefined}>
                    {entry.actor_email ?? "—"}
                  </span>
                </div>
                <strong>{actionLabels[entry.action] ?? entry.action}</strong>
                <span className="ellipsis audit-card-target" title={target(entry)}>
                  {target(entry)}
                </span>
              </article>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
