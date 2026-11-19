import { FormEvent, useState } from "react";
import { ArrowLeft } from "lucide-react";
import { User, VpnKey } from "../api";
import { formatBytes, formatDate, formatSeen } from "../utils";
import { CreateKeyForm } from "./CreateKeyForm";
import { Status } from "./Status";

/** Карточка пользователя в админке: кто это, что с его ключами и что с ним можно сделать. */
export function UserCard({
  user,
  keys,
  isSelf,
  onBack,
  onApprove,
  onSuspend,
  onRestore,
  onSaveLimit,
  onCreateKey
}: {
  user: User;
  keys: VpnKey[];
  isSelf: boolean;
  /** Кнопка «назад» к списку (только на телефоне). */
  onBack?: () => void;
  onApprove: () => void;
  onSuspend: () => void;
  onRestore: () => void;
  onSaveLimit: (value: number) => Promise<void>;
  onCreateKey: (name: string) => Promise<void>;
}) {
  const live = keys.filter((key) => key.status !== "revoked");
  const active = live.filter((key) => key.status === "active").length;
  const online = live.filter((key) => key.online).length;
  const down = keys.reduce((sum, key) => sum + key.traffic_down, 0);
  const up = keys.reduce((sum, key) => sum + key.traffic_up, 0);
  const lastSeen = keys.reduce<string | null>((latest, key) => {
    if (!key.last_seen_at) return latest;
    return !latest || new Date(key.last_seen_at) > new Date(latest) ? key.last_seen_at : latest;
  }, null);
  const activity = online > 0 ? "сейчас в сети" : lastSeen ? formatSeen(lastSeen) : "не подключался";

  return (
    <section className="selected-user-panel user-card">
      {onBack && (
        <button type="button" className="back-link" onClick={onBack}>
          <ArrowLeft size={18} /> Пользователи
        </button>
      )}
      <header className="user-card-head">
        <span className="user-avatar" aria-hidden="true">
          {user.email.slice(0, 1).toUpperCase()}
        </span>
        <div className="user-card-id">
          <h2 className="ellipsis" title={user.email}>
            {user.email}
          </h2>
          <div className="user-card-sub">
            <Status status={user.status} kind="user" />
            {isSelf && <span className="status approved">Вы</span>}
            <span>
              {user.role === "admin" ? "Администратор" : "Пользователь"} · с {formatDate(user.created_at)}
            </span>
          </div>
        </div>
        <div className="user-card-actions">
          {user.status === "pending" && (
            <button className="primary" onClick={onApprove}>
              Подтвердить
            </button>
          )}
          {!isSelf && user.status !== "suspended" && (
            <button className="ghost danger" onClick={onSuspend}>
              {user.status === "pending" ? "Отклонить" : "Заблокировать"}
            </button>
          )}
          {user.status === "suspended" && (
            <button className="primary" onClick={onRestore}>
              Восстановить
            </button>
          )}
        </div>
      </header>

      <dl className="user-stats">
        <div>
          <dt>Ключи</dt>
          <dd>
            {active} <small>из {user.key_limit}</small>
          </dd>
        </div>
        <div>
          <dt>В сети</dt>
          <dd>{online}</dd>
        </div>
        <div>
          <dt>Трафик</dt>
          <dd>
            ↓ {formatBytes(down)} <small>↑ {formatBytes(up)}</small>
          </dd>
        </div>
        <div>
          <dt>Активность</dt>
          <dd className="user-stats-text">{activity}</dd>
        </div>
      </dl>

      <div className="user-card-forms">
        <LimitForm key={`${user.id}-${user.key_limit}`} user={user} onSave={onSaveLimit} />
        <div className="user-card-form">
          <span className="form-title">Новый ключ</span>
          {user.status === "approved" ? (
            <CreateKeyForm key={user.id} placeholder="Название устройства" submitLabel="Добавить" onCreate={onCreateKey} />
          ) : (
            <p className="muted">
              {user.status === "pending" ? "Сначала подтвердите аккаунт." : "Сначала восстановите аккаунт."}
            </p>
          )}
        </div>
      </div>
    </section>
  );
}

function LimitForm({ user, onSave }: { user: User; onSave: (value: number) => Promise<void> }) {
  const [draft, setDraft] = useState(String(user.key_limit));
  const [busy, setBusy] = useState(false);
  const value = Number(draft);
  const valid = draft !== "" && Number.isInteger(value) && value >= 0 && value <= 100;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!valid || value === user.key_limit) return;
    setBusy(true);
    try {
      await onSave(value);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="user-card-form" onSubmit={submit}>
      <label className="form-title" htmlFor={`limit-${user.id}`}>
        Лимит ключей
      </label>
      <div className="limit-row">
        <input
          id={`limit-${user.id}`}
          className="limit-input"
          type="number"
          inputMode="numeric"
          min={0}
          max={100}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
        <button className="ghost" disabled={busy || !valid || value === user.key_limit}>
          Сохранить
        </button>
      </div>
    </form>
  );
}
