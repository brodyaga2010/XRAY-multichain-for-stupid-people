import { useEffect, useMemo, useState } from "react";
import { Activity, Clock3, KeyRound, ShieldAlert, Users } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { StatCard } from "../components/StatCard";
import { Status } from "../components/Status";
import { formatDate, matchesSearch } from "../utils";

export function AdminDashboard({ setError }: { setError: (error: string) => void }) {
  const [users, setUsers] = useState<User[]>([]);
  const [keys, setKeys] = useState<VpnKey[]>([]);
  const [keyQuery, setKeyQuery] = useState("");

  useEffect(() => {
    Promise.all([api.adminUsers(), api.adminKeys()])
      .then(([loadedUsers, loadedKeys]) => {
        setUsers(loadedUsers);
        setKeys(loadedKeys);
      })
      .catch((err) => setError(err.message));
  }, []);

  const stats = useMemo(() => {
    const activeKeys = keys.filter((key) => key.status === "active").length;
    const pendingUsers = users.filter((user) => user.status === "pending").length;
    const suspendedUsers = users.filter((user) => user.status === "suspended").length;
    const totalCapacity = users.reduce((sum, user) => sum + user.key_limit, 0);
    return { activeKeys, pendingUsers, suspendedUsers, totalCapacity };
  }, [users, keys]);

  const filteredKeys = useMemo(
    () => keys.filter((key) => matchesSearch([key.name, key.uuid, key.sni, key.type, key.status], keyQuery)),
    [keys, keyQuery]
  );

  const recentUsers = users.slice(0, 5);

  return (
    <div className="dashboard-grid">
      <div className="metrics-strip dashboard-metrics">
        <StatCard label="Active keys" value={stats.activeKeys} helper="Currently provisioned" icon={KeyRound} tone="good" />
        <StatCard label="Pending users" value={stats.pendingUsers} helper="Awaiting approval" icon={Clock3} tone="warn" />
        <StatCard label="Suspended" value={stats.suspendedUsers} helper="Paused accounts" icon={ShieldAlert} tone="danger" />
        <StatCard label="Capacity" value={stats.totalCapacity} helper="Total key allowance" icon={Users} />
      </div>

      <section className="table-panel animate-in">
        <div className="panel-heading">
          <h2>Recent users</h2>
          <span>{users.length} total</span>
        </div>
        <div className="activity-list">
          {recentUsers.map((user) => (
            <div className="activity-row" key={user.id}>
              <Activity size={17} />
              <div>
                <strong>{user.email}</strong>
                <span>Joined {formatDate(user.created_at)}</span>
              </div>
              <Status status={user.status} />
            </div>
          ))}
        </div>
      </section>

      <section className="dashboard-keys">
        <div className="toolbar-panel">
          <SearchBox value={keyQuery} onChange={setKeyQuery} placeholder="Search all keys..." />
        </div>
        <KeyTable
          keys={filteredKeys}
          onRevoke={async (id) => {
            await api.adminRevokeKey(id);
            setKeys(await api.adminKeys());
          }}
        />
      </section>
    </div>
  );
}
