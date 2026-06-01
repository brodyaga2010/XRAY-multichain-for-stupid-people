import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock3, ShieldAlert, Users } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { StatCard } from "../components/StatCard";
import { Status } from "../components/Status";
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
      if (!current) return loaded[0] ?? null;
      return loaded.find((item) => item.id === current.id) ?? loaded[0] ?? null;
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
          onApprove={(id) => mutateUser(() => api.approveUser(id))}
          onSuspend={(id) => mutateUser(() => api.suspendUser(id))}
          onRestore={(id) => mutateUser(() => api.restoreUser(id))}
          onSetLimit={setLimit}
        />
      </div>
      <div className="stack">
        {selected ? (
          <>
            <div className="detail-heading">
              <div>
                <p className="eyebrow">Keys for</p>
                <h2>{selected.email}</h2>
              </div>
              <Status status={selected.status} />
            </div>
            <CreateKeyForm
              user={selected}
              compact
              onCreate={async (payload) => {
                await api.adminCreateKey(selected.id, payload);
                await loadKeys(selected.id);
              }}
            />
            <KeyTable
              keys={keys}
              onRevoke={async (id) => {
                await api.adminRevokeKey(id);
                await loadKeys(selected.id);
              }}
            />
          </>
        ) : (
          <div className="empty-panel">No users found</div>
        )}
      </div>
    </section>
  );
}

function UserPicker({
  users,
  currentUser,
  selected,
  onSelect,
  onApprove,
  onSuspend,
  onRestore,
  onSetLimit
}: {
  users: User[];
  currentUser: User;
  selected: User | null;
  onSelect: (user: User) => void;
  onApprove: (id: number) => void;
  onSuspend: (id: number) => void;
  onRestore: (id: number) => void;
  onSetLimit: (id: number, value: number) => void;
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
            <span className="user-main">
              <strong>{item.email}</strong>
              <Status status={item.status} />
            </span>
            <span className="user-controls" onClick={(event) => event.stopPropagation()}>
              <label>
                Limit
                <input
                  className="limit-input"
                  type="number"
                  min={0}
                  max={100}
                  value={item.key_limit}
                  onChange={(event) => onSetLimit(item.id, Number(event.target.value))}
                />
              </label>
              {item.status === "pending" && (
                <button className="small" onClick={() => onApprove(item.id)}>
                  Approve
                </button>
              )}
              {item.id !== currentUser.id && item.status !== "suspended" && (
                <button className="small ghost" onClick={() => onSuspend(item.id)}>
                  Suspend
                </button>
              )}
              {item.status === "suspended" && (
                <button className="small" onClick={() => onRestore(item.id)}>
                  Restore
                </button>
              )}
            </span>
          </div>
        ))}
        {users.length === 0 && <div className="empty">No users found</div>}
      </div>
    </div>
  );
}
