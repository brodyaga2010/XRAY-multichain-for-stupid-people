import { useEffect, useMemo, useState } from "react";
import { ArrowUpDown, Clock3, KeyRound, ShieldAlert, Users, Wifi } from "lucide-react";
import { api, MonitorStatus, User, VpnKey } from "@/api";
import { HealthPanel } from "@/components/HealthPanel";
import { StatCard } from "@/components/StatCard";
import { StatusBadge } from "@/components/StatusBadge";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useAutoRefresh } from "@/hooks";
import { AppView } from "@/types";
import { formatBytes, formatDate, keyTitle } from "@/utils";

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
    <div className="grid gap-4 md:gap-6">
      <div className="grid grid-cols-2 gap-3 md:gap-4 lg:grid-cols-3">
        <StatCard label="Активные ключи" value={stats.activeKeys} helper="Выданы и работают" icon={KeyRound} tone="good" />
        <StatCard label="Сейчас в сети" value={stats.onlineKeys} helper="Ключей онлайн" icon={Wifi} tone="good" />
        <StatCard label="Трафик всего" value={formatBytes(stats.totalTraffic)} helper="Загрузка + отдача" icon={ArrowUpDown} />
        <StatCard label="Ждут подтверждения" value={stats.pendingUsers} helper="Новые пользователи" icon={Clock3} tone="warn" />
        <StatCard label="Заблокированы" value={stats.suspendedUsers} helper="Доступ приостановлен" icon={ShieldAlert} tone="danger" />
        <StatCard label="Лимит ключей" value={stats.totalCapacity} helper="Сумма лимитов" icon={Users} />
      </div>

      <HealthPanel status={health} />

      <div className="grid items-start gap-4 md:gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Ждут подтверждения</CardTitle>
            <CardDescription>ожидают: {stats.pendingUsers}</CardDescription>
          </CardHeader>
          <CardContent>
            {pendingUsers.length > 0 ? (
              <ul className="grid divide-y">
                {pendingUsers.map((user) => (
                  <li key={user.id} className="flex items-center gap-3 py-2.5 first:pt-0 last:pb-0">
                    <UserAvatar email={user.email} />
                    <div className="grid min-w-0 flex-1">
                      <span className="truncate text-sm font-medium" title={user.email}>
                        {user.email}
                      </span>
                      <span className="text-xs text-muted-foreground">Запрос: {formatDate(user.created_at)}</span>
                    </div>
                    <Button disabled={approving === user.id} onClick={() => approve(user.id)}>
                      Подтвердить
                    </Button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">Нет заявок на подтверждение</p>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Новые пользователи</CardTitle>
            <CardDescription>всего: {users.length}</CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="grid divide-y">
              {recentUsers.map((user) => (
                <li key={user.id} className="flex items-center gap-3 py-2.5 first:pt-0 last:pb-0">
                  <UserAvatar email={user.email} />
                  <div className="grid min-w-0 flex-1">
                    <span className="truncate text-sm font-medium" title={user.email}>
                      {user.email}
                    </span>
                    <span className="text-xs text-muted-foreground">Регистрация: {formatDate(user.created_at)}</span>
                  </div>
                  <StatusBadge status={user.status} kind="user" />
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Последние ключи</CardTitle>
          <CardDescription>всего: {keys.length}</CardDescription>
          {keys.length > 0 && (
            <CardAction>
              <Button variant="outline" onClick={() => onNavigate("keys")}>
                Все ключи
              </Button>
            </CardAction>
          )}
        </CardHeader>
        <CardContent>
          {recentKeys.length > 0 ? (
            <ul className="grid divide-y">
              {recentKeys.map((key) => (
                <li key={key.id} className="flex items-center gap-3 py-2.5 first:pt-0 last:pb-0">
                  <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">
                    <KeyRound className="size-4" />
                  </span>
                  <div className="grid min-w-0 flex-1">
                    <span className="truncate text-sm font-medium" title={keyTitle(key)}>
                      {keyTitle(key)}
                    </span>
                    <span className="truncate text-xs text-muted-foreground" title={key.owner_email ?? undefined}>
                      {key.owner_email ?? "—"} · {formatDate(key.created_at)}
                    </span>
                  </div>
                  <StatusBadge status={key.status} />
                </li>
              ))}
            </ul>
          ) : (
            <p className="py-6 text-center text-sm text-muted-foreground">Ключей пока нет</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
