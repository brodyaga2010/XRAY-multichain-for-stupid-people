import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock3, Plus, ShieldAlert, Users } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { StatCard } from "../components/StatCard";
import { Status } from "../components/Status";
import { useMediaQuery } from "../hooks";
import { matchesSearch } from "../utils";

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
      setError(err instanceof Error ? err.message : "Failed to update user");
    }
  }

  async function setLimit(userId: number, value: number) {
    await mutateUser(() => api.updateLimit(userId, value));
  }

  return (
    <section className="admin-layout">
      {(!isMobile || !selected) && (
        <div className="stack">
          <div className="metrics-strip non-dashboard-metrics">
            <StatCard label="Approved" value={stats.approved} helper="Can sign in" icon={CheckCircle2} tone="good" />
            <StatCard label="Pending" value={stats.pending} helper="Need approval" icon={Clock3} tone="warn" />
            <StatCard label="Suspended" value={stats.suspended} helper="Access paused" icon={ShieldAlert} tone="danger" />
            <StatCard label="Users" value={users.length} helper="Total accounts" icon={Users} />
          </div>
          <div className="toolbar-panel">
            <SearchBox value={query} onChange={setQuery} placeholder="Search users by email, status, role..." />
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
              <div>
                <p className="eyebrow">Selected user</p>
                <h2>{selected.email}</h2>
              </div>
              <div className="detail-actions">
                <Status status={selected.status} />
                {isMobile && (
                  <button
                    type="button"
                    className="small ghost"
                    onClick={() => {
                      setSelected(null);
                      setShowCreate(false);
                    }}
                  >
                    Change user
                  </button>
                )}
              </div>
            </div>
            <div className="selected-user-controls">
              <label className="limit-field compact">
                <span>Limit</span>
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
                  <button className="small" onClick={() => mutateUser(() => api.approveUser(selected.id))}>
                    Approve
                  </button>
                )}
                {selected.id !== currentUser.id && selected.status !== "suspended" && (
                  <button className="small ghost" onClick={() => mutateUser(() => api.suspendUser(selected.id))}>
                    Suspend
                  </button>
                )}
                {selected.status === "suspended" && (
                  <button className="small" onClick={() => mutateUser(() => api.restoreUser(selected.id))}>
                    Restore
                  </button>
                )}
              </div>
              {isMobile ? (
                showCreate ? (
                  <CreateKeyForm
                    user={selected}
                    compact
                    onClose={() => setShowCreate(false)}
                    onCreate={async (payload) => {
                      await api.adminCreateKey(selected.id, payload);
                      await loadKeys(selected.id);
                      setShowCreate(false);
                    }}
                  />
                ) : (
                  <button className="primary mobile-create-button" onClick={() => setShowCreate(true)}>
                    <Plus size={17} />
                    New key
                  </button>
                )
              ) : (
                <CreateKeyForm
                  user={selected}
                  compact
                  onCreate={async (payload) => {
                    await api.adminCreateKey(selected.id, payload);
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
        <div className="empty-panel">Choose a user to manage access and keys</div>
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
        <h2>Users</h2>
        <span>{users.length} shown</span>
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
              <strong>{item.email}</strong>
              <Status status={item.status} />
            </div>
          </div>
        ))}
        {users.length === 0 && <div className="empty">No users found</div>}
      </div>
    </div>
  );
}
