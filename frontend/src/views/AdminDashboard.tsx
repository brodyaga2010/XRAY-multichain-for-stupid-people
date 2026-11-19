import { useEffect, useMemo, useState } from "react";
import { Activity, ArrowUpDown, Clock3, KeyRound, ShieldAlert, Users, Wifi } from "lucide-react";
import { api, MonitorStatus, User, VpnKey } from "../api";
import { HealthPanel } from "../components/HealthPanel";
import { StatCard } from "../components/StatCard";
import { Status } from "../components/Status";
import { useAutoRefresh } from "../hooks";
import { AppView } from "../types";
import { formatBytes, formatDate, keyTitle } from "../utils";

export function AdminDashboard({
  setError,
  onNavigate
}: {
  setError: (error: string) => void;
  onNavigate: (view: AppView) => void;
}) {
  const [users, setUsers] = useState<User[]>([]);
  const [keys, setKeys] = useState<VpnKey[]>([]);
  const [health, setHealth] = useState<MonitorStatus | null>(null);
  const [approving, setApproving] = useState<number | null>(null);

  async function load() {
    // Состояние грузится отдельно: его ошибка не должна прятать пользователей и ключи.
    const status = api.adminStatus().then(setHealth, () => setHealth(null));
    const [loadedUsers, loadedKeys] = await Promise.all([api.adminUsers(), api.adminKeys()]);
    setUsers(loadedUsers);
    setKeys(loadedKeys);
    await status;
  }

  useEffect(() => {
    load().catch((err) => setError(err.message));
  }, []);

  useAutoRefresh(load);

  async function approve(userId: number) {
    setApproving(userId);
    setError("");
    try {
      await api.approveUser(userId);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось подтвердить пользователя");
    } finally {
      setApproving(null);
    }
  }

  const stats = useMemo(() => {
    const activeKeys = keys.filter((key) => key.status === "active").length;
    const pendingUsers = users.filter((user) => user.status === "pending").length;
    const suspendedUsers = users.filter((user) => user.status === "suspended").length;
    const totalCapacity = users.reduce((sum, user) => sum + user.key_limit, 0);
    const onlineKeys = keys.filter((key) => key.online).length;
    const totalTraffic = keys.reduce((sum, key) => sum + key.traffic_up + key.traffic_down, 0);
    return { activeKeys, pendingUsers, suspendedUsers, totalCapacity, onlineKeys, totalTraffic };
  }, [users, keys]);

  const recentUsers = users.slice(0, 5);
  const pendingUsers = users.filter((user) => user.status === "pending").slice(0, 5);
  const recentKeys = [...keys]
    .sort((left, right) => new Date(right.created_at).getTime() - new Date(left.created_at).getTime())
    .slice(0, 6);

  return (
    <div className="dashboard-grid">
      <div className="metrics-strip dashboard-metrics">
        <StatCard label="Активные ключи" value={stats.activeKeys} helper="Выданы и работают" icon={KeyRound} tone="good" />
        <StatCard label="Сейчас в сети" value={stats.onlineKeys} helper="Ключей онлайн" icon={Wifi} tone="good" />
        <StatCard label="Трафик всего" value={formatBytes(stats.totalTraffic)} helper="Загрузка + отдача" icon={ArrowUpDown} />
        <StatCard label="Ждут подтверждения" value={stats.pendingUsers} helper="Новые пользователи" icon={Clock3} tone="warn" />
        <StatCard label="Заблокированы" value={stats.suspendedUsers} helper="Доступ приостановлен" icon={ShieldAlert} tone="danger" />
        <StatCard label="Лимит ключей" value={stats.totalCapacity} helper="Сумма лимитов" icon={Users} />
      </div>

      <HealthPanel status={health} />

      <section className="table-panel animate-in">
        <div className="panel-heading">
          <h2>Ждут подтверждения</h2>
          <span>ожидают: {stats.pendingUsers}</span>
        </div>
        <div className="activity-list">
          {pendingUsers.length > 0 ? (
            pendingUsers.map((user) => (
              <div className="activity-row" key={user.id}>
                <Clock3 size={17} />
                <div>
                  <strong className="ellipsis" title={user.email}>
                    {user.email}
                  </strong>
                  <span>Запрос доступа: {formatDate(user.created_at)}</span>
                </div>
                <button className="small" disabled={approving === user.id} onClick={() => approve(user.id)}>
                  Подтвердить
                </button>
              </div>
            ))
          ) : (
            <div className="empty">Нет заявок на подтверждение</div>
          )}
        </div>
      </section>
      <section className="table-panel animate-in">
        <div className="panel-heading">
          <h2>Новые пользователи</h2>
          <span>всего: {users.length}</span>
        </div>
        <div className="activity-list">
          {recentUsers.map((user) => (
            <div className="activity-row" key={user.id}>
              <Activity size={17} />
              <div>
                <strong className="ellipsis" title={user.email}>
                  {user.email}
                </strong>
                <span>Регистрация: {formatDate(user.created_at)}</span>
                <Status status={user.status} kind="user" />
              </div>
            </div>
          ))}
        </div>
      </section>
      <section className="table-panel dashboard-keys animate-in">
        <div className="panel-heading">
          <h2>Последние ключи</h2>
          <span>всего: {keys.length}</span>
        </div>
        <div className="activity-list">
          {recentKeys.map((key) => (
            <div className="activity-row" key={key.id}>
              <KeyRound size={17} />
              <div>
                <strong className="ellipsis" title={keyTitle(key)}>
                  {keyTitle(key)}
                </strong>
                <span className="ellipsis" title={key.owner_email ?? undefined}>
                  {key.owner_email ?? "—"} / {formatDate(key.created_at)}
                </span>
              </div>
              <Status status={key.status} />
            </div>
          ))}
          {recentKeys.length === 0 && <div className="empty">Ключей пока нет</div>}
        </div>
        {keys.length > 0 && (
          <button type="button" className="link-button" onClick={() => onNavigate("keys")}>
            Все ключи
          </button>
        )}
      </section>
    </div>
  );
}
