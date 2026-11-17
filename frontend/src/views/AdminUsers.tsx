import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, CheckCircle2, ChevronRight, Clock3, Eye, EyeOff, Plus, ShieldAlert, Users } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { StatCard } from "../components/StatCard";
import { Status } from "../components/Status";
import { useMediaQuery } from "../hooks";
import { formatDate, matchesSearch } from "../utils";

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
  const [showCreate, setShowCreate] = useState(false);
  const [showWidgets, setShowWidgets] = useState(true);
  const isMobile = useMediaQuery("(max-width: 820px)");

  const filteredUsers = useMemo(
    () => users.filter((user) => matchesSearch([user.email, user.status, user.role, user.key_limit], query)),
    [users, query]
  );

  const stats = useMemo(
    () => ({
      approved: users.filter((item) => item.status === "approved").length,
      pending: users.filter((item) => item.status === "pending").length,
      suspended: users.filter((item) => item.status === "suspended").length
    }),
    [users]
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
    if (isMobile && showCreate && !selected) setShowCreate(false);
  }, [isMobile, selected, showCreate]);

  useEffect(() => {
    if (selected) loadKeys(selected.id).catch((err) => setError(err.message));
  }, [selected?.id]);

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

  async function setLimit(userId: number, value: number) {
    await mutateUser(() => api.updateLimit(userId, value));
  }

  return (
    <section className="admin-layout">
      {(!isMobile || !selected) && (
        <div className="stack">
          {showWidgets && (
            <div className="metrics-strip non-dashboard-metrics collapsible-widgets">
              <StatCard label="Подтверждены" value={stats.approved} helper="Могут входить" icon={CheckCircle2} tone="good" />
              <StatCard label="Ждут" value={stats.pending} helper="Нужно подтвердить" icon={Clock3} tone="warn" />
              <StatCard label="Заблокированы" value={stats.suspended} helper="Доступ приостановлен" icon={ShieldAlert} tone="danger" />
              <StatCard label="Пользователи" value={users.length} helper="Всего аккаунтов" icon={Users} />
            </div>
          )}
          <div className="toolbar-panel sticky-toolbar">
            <SearchBox value={query} onChange={setQuery} placeholder={isMobile ? "Поиск пользователей" : "Поиск по почте, статусу, роли..."} />
            <div className="toolbar-actions">
              <button className="small ghost widgets-toggle" type="button" onClick={() => setShowWidgets((current) => !current)}>
                {showWidgets ? <EyeOff size={15} /> : <Eye size={15} />}
                <span>{showWidgets ? "Скрыть виджеты" : "Показать виджеты"}</span>
              </button>
            </div>
          </div>
          <UserPicker
            users={filteredUsers}
            currentUser={currentUser}
            selected={selected}
            onSelect={setSelected}
          />
        </div>
      )}
      {selected ? (
        <div className="stack">
          <section className="selected-user-panel">
            <div className="selected-user-head">
              <div className="selected-user-title-row">
                {isMobile && (
                  <button
                    type="button"
                    className="icon-button"
                    onClick={() => {
                      setSelected(null);
                      setShowCreate(false);
                    }}
                    title="К списку пользователей"
                  >
                    <ArrowLeft size={16} />
                  </button>
                )}
                <div className="selected-user-copy">
                  <div className="selected-user-title-line">
                    <h2>{selected.email}</h2>
                    <Status status={selected.status} kind="user" />
                  </div>
                  <p className="selected-user-subtitle">Регистрация: {formatDate(selected.created_at)} / Лимит {selected.key_limit}</p>
                </div>
              </div>
            </div>
            <div className="selected-user-toolbar">
              <label className="limit-field compact">
                <span>Лимит</span>
                <input
                  className="limit-input"
                  type="number"
                  min={0}
                  max={100}
                  value={selected.key_limit}
                  onChange={(event) => setLimit(selected.id, Number(event.target.value))}
                />
              </label>
              <div className="user-buttons compact">
                {selected.status === "pending" && (
                  <button className="small action-chip" onClick={() => mutateUser(() => api.approveUser(selected.id))}>
                    Подтвердить
                  </button>
                )}
                {selected.id !== currentUser.id && selected.status !== "suspended" && (
                  <button className="small ghost action-chip" onClick={() => suspend(selected.id)}>
                    Заблокировать
                  </button>
                )}
                {selected.status === "suspended" && (
                  <button className="small action-chip" onClick={() => mutateUser(() => api.restoreUser(selected.id))}>
                    Восстановить
                  </button>
                )}
              </div>
            </div>
            <div className="selected-user-controls">
              {isMobile ? (
                showCreate ? (
                  <CreateKeyForm
                    user={selected}
                    compact
                    onClose={() => setShowCreate(false)}
                    onCreate={async (name) => {
                      await api.adminCreateKey(selected.id, name);
                      await loadKeys(selected.id);
                      setShowCreate(false);
                    }}
                  />
                ) : (
                  <button className="primary mobile-create-button" onClick={() => setShowCreate(true)}>
                    <Plus size={17} />
                    Новый ключ
                  </button>
                )
              ) : (
                <CreateKeyForm
                  user={selected}
                  compact
                  onCreate={async (name) => {
                    await api.adminCreateKey(selected.id, name);
                    await loadKeys(selected.id);
                  }}
                />
              )}
            </div>
          </section>
          <KeyTable
            keys={keys}
            onRevoke={async (id) => {
              await api.adminRevokeKey(id);
              await loadKeys(selected.id);
            }}
          />
        </div>
      ) : isMobile ? (
        <div className="empty-panel">Выберите пользователя, чтобы управлять доступом и ключами</div>
      ) : null}
    </section>
  );
}

function UserPicker({
  users,
  currentUser,
  selected,
  onSelect
}: {
  users: User[];
  currentUser: User;
  selected: User | null;
  onSelect: (user: User) => void;
}) {
  return (
    <div className="user-picker animate-in">
      <div className="panel-heading">
        <h2>Пользователи</h2>
        <span>показано: {users.length}</span>
      </div>
      <div className="user-list">
        {users.map((item) => (
          <div
            key={item.id}
            role="button"
            tabIndex={0}
            className={`user-row ${selected?.id === item.id ? "selected" : ""}`}
            onClick={() => onSelect(item)}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") onSelect(item);
            }}
          >
            <div className="user-main">
              <div className="user-row-top">
                <strong>{item.email}</strong>
                <div className="user-row-badges">
                  {item.id === currentUser.id && <span className="status approved">Вы</span>}
                  <Status status={item.status} kind="user" />
                </div>
              </div>
              <span>{item.role === "admin" ? "Администратор" : "Пользователь"} / Лимит {item.key_limit} / {formatDate(item.created_at)}</span>
            </div>
            <ChevronRight size={16} className="user-row-arrow" />
          </div>
        ))}
        {users.length === 0 && <div className="empty">Пользователи не найдены</div>}
      </div>
    </div>
  );
}
