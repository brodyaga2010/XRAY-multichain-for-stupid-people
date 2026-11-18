import { FormEvent, useEffect, useMemo, useState } from "react";
import { ArrowLeft, ChevronRight } from "lucide-react";
import { api, User, UserStatus, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { FilterChips } from "../components/FilterChips";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { Status } from "../components/Status";
import { useAutoRefresh, useMediaQuery } from "../hooks";
import { formatDate, matchesSearch } from "../utils";

type UserFilter = "all" | UserStatus;

const userFilters: Array<{ id: UserFilter; label: string }> = [
  { id: "all", label: "Все" },
  { id: "pending", label: "Ждут" },
  { id: "approved", label: "Подтверждены" },
  { id: "suspended", label: "Заблокированы" }
];

export function AdminUsers({
  currentUser,
  setError
}: {
  currentUser: User;
  setError: (error: string) => void;
}) {
  const [users, setUsers] = useState<User[]>([]);
  const [selected, setSelected] = useState<User | null>(null);
  const [keys, setKeys] = useState<VpnKey[]>([]);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<UserFilter>("all");
  const [newKeyId, setNewKeyId] = useState<number | null>(null);
  const isMobile = useMediaQuery("(max-width: 900px)");

  const filteredUsers = useMemo(
    () =>
      users.filter(
        (user) =>
          (filter === "all" || user.status === filter) &&
          matchesSearch([user.email, user.status, user.role, user.key_limit], query)
      ),
    [users, query, filter]
  );

  async function loadUsers() {
    const loaded = await api.adminUsers();
    setUsers(loaded);
    setSelected((current) => {
      const match = current ? loaded.find((item) => item.id === current.id) ?? null : null;
      if (match) return match;
      if (isMobile) return null;
      return loaded[0] ?? null;
    });
  }

  async function loadKeys(userId: number) {
    setKeys(await api.adminUserKeys(userId));
  }

  useEffect(() => {
    loadUsers().catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    if (selected) loadKeys(selected.id).catch((err) => setError(err.message));
  }, [selected?.id]);

  useAutoRefresh(() => Promise.all([loadUsers(), selected ? loadKeys(selected.id) : null]));

  function select(user: User | null) {
    setSelected(user);
    setNewKeyId(null);
    setKeys([]);
    if (isMobile) window.scrollTo({ top: 0 });
  }

  async function mutateUser(action: () => Promise<User>) {
    setError("");
    try {
      await action();
      await loadUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось обновить пользователя");
    }
  }

  async function suspend(userId: number) {
    if (!window.confirm("Ключи пользователя перестанут работать. Заблокировать?")) return;
    await mutateUser(() => api.suspendUser(userId));
  }

  async function revokeKey(id: number) {
    if (!selected) return;
    setError("");
    try {
      await api.adminRevokeKey(id);
      await loadKeys(selected.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось отозвать ключ");
    }
  }

  async function createKey(userId: number, name: string) {
    setError("");
    const created = await api.adminCreateKey(userId, name);
    await loadKeys(userId);
    setNewKeyId(created.id);
  }

  const showList = !isMobile || !selected;

  return (
    <section className="admin-layout">
      {showList && (
        <div className="user-picker">
          <div className="panel-heading">
            <h2>Пользователи</h2>
            <span>показано: {filteredUsers.length}</span>
          </div>
          <SearchBox value={query} onChange={setQuery} placeholder="Поиск по почте, статусу, роли..." />
          <FilterChips label="Фильтр по статусу" options={userFilters} value={filter} onChange={setFilter} />
          <div className="user-list">
            {filteredUsers.map((item) => (
              <div
                key={item.id}
                role="button"
                tabIndex={0}
                className={`user-row ${selected?.id === item.id ? "selected" : ""}`}
                onClick={() => select(item)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    select(item);
                  }
                }}
              >
                <div className="user-main">
                  <div className="user-row-top">
                    <strong className="ellipsis" title={item.email}>
                      {item.email}
                    </strong>
                    <div className="user-row-badges">
                      {item.id === currentUser.id && <span className="status approved">Вы</span>}
                      <Status status={item.status} kind="user" />
                    </div>
                  </div>
                  <span className="ellipsis">
                    {item.role === "admin" ? "Администратор" : "Пользователь"} / лимит {item.key_limit} /{" "}
                    {formatDate(item.created_at)}
                  </span>
                </div>
                <ChevronRight size={16} className="user-row-arrow" />
              </div>
            ))}
            {filteredUsers.length === 0 && <div className="empty">Пользователи не найдены</div>}
          </div>
        </div>
      )}
      {selected ? (
        <div className="stack">
          <section className="selected-user-panel">
            {isMobile && (
              <button type="button" className="back-link" onClick={() => select(null)}>
                <ArrowLeft size={18} /> Пользователи
              </button>
            )}
            <h2 className="ellipsis user-card-email" title={selected.email}>
              {selected.email}
            </h2>
            <dl className="user-facts">
              <div>
                <dt>Роль</dt>
                <dd>{selected.role === "admin" ? "Администратор" : "Пользователь"}</dd>
              </div>
              <div>
                <dt>Регистрация</dt>
                <dd>{formatDate(selected.created_at)}</dd>
              </div>
              <div>
                <dt>Статус</dt>
                <dd>
                  <Status status={selected.status} kind="user" />
                </dd>
              </div>
            </dl>
            <div className="action-row">
              {selected.status === "pending" && (
                <button className="primary" onClick={() => mutateUser(() => api.approveUser(selected.id))}>
                  Подтвердить
                </button>
              )}
              {selected.id !== currentUser.id && selected.status !== "suspended" && (
                <button className="ghost danger" onClick={() => suspend(selected.id)}>
                  Заблокировать
                </button>
              )}
              {selected.status === "suspended" && (
                <button className="primary" onClick={() => mutateUser(() => api.restoreUser(selected.id))}>
                  Восстановить
                </button>
              )}
            </div>
            <LimitForm
              key={`${selected.id}-${selected.key_limit}`}
              user={selected}
              onSave={(value) => mutateUser(() => api.updateLimit(selected.id, value))}
            />
            <div className="user-section">
              <h3>Добавить ключ</h3>
              {selected.status === "approved" ? (
                <CreateKeyForm
                  key={selected.id}
                  placeholder="Название устройства"
                  submitLabel="Добавить"
                  onCreate={(name) => createKey(selected.id, name)}
                />
              ) : (
                <p className="muted">
                  Ключи можно добавлять только подтверждённым пользователям.{" "}
                  {selected.status === "pending" ? "Сначала подтвердите аккаунт." : "Сначала восстановите аккаунт."}
                </p>
              )}
            </div>
          </section>
          <KeyTable keys={keys} onRevoke={revokeKey} autoOpenKeyId={newKeyId} />
        </div>
      ) : (
        !isMobile && <div className="empty-panel">Выберите пользователя, чтобы управлять доступом и ключами</div>
      )}
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
    <form className="limit-form user-section" onSubmit={submit}>
      <label>
        Лимит ключей
        <input
          className="limit-input"
          type="number"
          inputMode="numeric"
          min={0}
          max={100}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
      </label>
      <button className="ghost" disabled={busy || !valid || value === user.key_limit}>
        Сохранить
      </button>
    </form>
  );
}
