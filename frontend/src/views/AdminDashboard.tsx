import { useEffect, useMemo, useState } from "react";
import { Activity, ArrowUpDown, Check, Clock3, KeyRound, Users, Wifi } from "lucide-react";
import { api, MonitorStatus, User, VpnKey } from "@/api";
import { overall } from "@/components/HealthPanel";
import { Stat, StatStrip } from "@/components/StatStrip";
import { StatusBadge } from "@/components/StatusBadge";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
    const onlineKeys = keys.filter((key) => key.online).length;
    const totalTraffic = keys.reduce((sum, key) => sum + key.traffic_up + key.traffic_down, 0);
    return { activeKeys, pendingUsers, suspendedUsers, onlineKeys, totalTraffic };
  }, [users, keys]);

  // Сначала заявки (их можно подтвердить прямо здесь), остальное место — новые пользователи.
  const byNewest = [...users].sort((left, right) => right.created_at.localeCompare(left.created_at));
  const shownUsers = [
    ...byNewest.filter((user) => user.status === "pending"),
    ...byNewest.filter((user) => user.status !== "pending")
  ].slice(0, 5);
  // Подробности состояния — на отдельной вкладке; здесь только итог одной ячейкой.
  const vpn: Stat = (() => {
    const base = { label: "Состояние VPN", icon: Activity, onClick: () => onNavigate("status") };
    if (!health || !health.enabled) return { ...base, value: "—" };
    const { tone } = overall(health);
    const short = { ok: "Работает", warn: "Перебои", fail: "Сбой", unknown: "Проверка…" }[tone];
    const toneMap = { ok: "good", warn: "warn", fail: "danger", unknown: "neutral" } as const;
    return { ...base, value: short, tone: toneMap[tone] };
  })();
  const recentKeys = [...keys].sort((left, right) => right.created_at.localeCompare(left.created_at)).slice(0, 5);

  return (
    <div className="grid gap-4 md:gap-6">
      <StatStrip
        items={[
          vpn,
          { label: "Активные ключи", value: stats.activeKeys, icon: KeyRound, tone: "good", onClick: () => onNavigate("keys") },
          { label: "Сейчас в сети", value: stats.onlineKeys, icon: Wifi, tone: "good", onClick: () => onNavigate("keys") },
          { label: "Трафик всего", value: formatBytes(stats.totalTraffic), icon: ArrowUpDown },
          { label: "Заявки", value: stats.pendingUsers, icon: Clock3, tone: "warn", onClick: () => onNavigate("admin") },
          { label: "Пользователей", value: users.length, icon: Users, onClick: () => onNavigate("admin") }
        ]}
      />

      <div className="grid items-start gap-4 md:gap-6 lg:grid-cols-2">
        <Card className="gap-3">
          <CardHeader>
            <CardTitle>Пользователи</CardTitle>
            <CardAction>
              <Button variant="ghost" className="-my-1.5 text-muted-foreground" onClick={() => onNavigate("admin")}>
                Все
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            {shownUsers.length > 0 ? (
              <ul className="grid divide-y">
                {shownUsers.map((user) => (
                  <li key={user.id} className="flex items-center gap-3 py-2 first:pt-0 last:pb-0">
                    <UserAvatar email={user.email} />
                    <div className="grid min-w-0 flex-1">
                      <span className="truncate text-sm font-medium" title={user.email}>
                        {user.email}
                      </span>
                      <span className="text-xs text-muted-foreground">с {formatDate(user.created_at)}</span>
                    </div>
                    {/* У заявки на узком экране хватает кнопки ✓ — длинная плашка съедала бы почту. */}
                    <StatusBadge status={user.status} kind="user" className={user.status === "pending" ? "max-sm:hidden" : undefined} />
                    {user.status === "pending" && (
                      <Button
                        size="icon-sm"
                        variant="outline"
                        className="text-success hover:bg-success/10 hover:text-success"
                        disabled={approving === user.id}
                        onClick={() => approve(user.id)}
                        aria-label={`Подтвердить ${user.email}`}
                        title="Подтвердить"
                      >
                        <Check />
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">Пользователей пока нет</p>
            )}
          </CardContent>
        </Card>

      <Card className="gap-3">
        <CardHeader>
          <CardTitle>Последние ключи</CardTitle>
          <CardAction>
            <Button variant="ghost" className="-my-1.5 text-muted-foreground" onClick={() => onNavigate("keys")}>
              Все
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          {recentKeys.length > 0 ? (
            <ul className="grid divide-y">
              {recentKeys.map((key) => (
                <li key={key.id} className="flex items-center gap-3 py-2 first:pt-0 last:pb-0">
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
    </div>
  );
}
