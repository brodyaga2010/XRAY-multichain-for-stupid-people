import { useEffect, useState } from "react";
import { api, AuditEntry } from "../api";
import { formatDateTime } from "../utils";

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

function target(entry: AuditEntry) {
  if (!entry.target_type) return "—";
  return entry.target_id !== null && entry.target_id !== undefined
    ? `${entry.target_type} #${entry.target_id}`
    : entry.target_type;
}

export function AuditLog({ setError }: { setError: (error: string) => void }) {
  const [entries, setEntries] = useState<AuditEntry[]>([]);

  useEffect(() => {
    api
      .adminAudit(200)
      .then(setEntries)
      .catch((err) => setError(err.message));
  }, []);

  return (
    <div className="table-panel animate-in">
      <div className="panel-heading">
        <h2>Журнал</h2>
        <span>записей: {entries.length}</span>
      </div>
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
                <td data-label="Дата">{formatDateTime(entry.created_at)}</td>
                <td data-label="Кто">{entry.actor_email ?? "—"}</td>
                <td data-label="Действие">{actionLabels[entry.action] ?? entry.action}</td>
                <td data-label="Объект">{target(entry)}</td>
              </tr>
            ))}
            {entries.length === 0 && (
              <tr>
                <td colSpan={4} className="empty">
                  Записей нет
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
