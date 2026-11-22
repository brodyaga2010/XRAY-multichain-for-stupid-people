import { FormEvent, useCallback, useState } from "react";
import { api, Plan, SubscriptionChange, User, VpnKey } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { formatBytes, formatDate, formatSeen } from "@/utils";
import { CreateKeyForm } from "./CreateKeyForm";
import { StatusBadge } from "./StatusBadge";
import { SubscriptionForm } from "./SubscriptionForm";
import { TrafficChart } from "./TrafficChart";
import { UserAvatar } from "./UserAvatar";

/** Карточка пользователя в админке: кто это, что с его ключами и что с ним можно сделать. */
export function UserCard({
  user,
  keys,
  isSelf,
  className,
  onApprove,
  onSuspend,
  onRestore,
  onSaveLimit,
  onCreateKey,
  plans,
  onChangeSubscription
}: {
  user: User;
  keys: VpnKey[];
  isSelf: boolean;
  className?: string;
  onApprove: () => void;
  onSuspend: () => void;
  onRestore: () => void;
  onSaveLimit: (value: number) => Promise<void>;
  onCreateKey: (name: string) => Promise<void>;
  plans: Plan[];
  onChangeSubscription: (change: SubscriptionChange) => Promise<void>;
}) {
  const live = keys.filter((key) => key.status !== "revoked");
  const active = live.filter((key) => key.status === "active").length;
  const online = live.filter((key) => key.online).length;
  const loadTraffic = useCallback((days: number) => api.adminTraffic(days, user.id), [user.id]);
  const down = keys.reduce((sum, key) => sum + key.traffic_down, 0);
  const up = keys.reduce((sum, key) => sum + key.traffic_up, 0);
  const lastSeen = keys.reduce<string | null>((latest, key) => {
    if (!key.last_seen_at) return latest;
    return !latest || new Date(key.last_seen_at) > new Date(latest) ? key.last_seen_at : latest;
  }, null);
  const activity = online > 0 ? "сейчас в сети" : lastSeen ? formatSeen(lastSeen) : "не подключался";

  const stats = [
    { label: "Ключи", value: active, extra: `из ${user.key_limit}` },
    { label: "В сети", value: online },
    { label: "Трафик", value: `↓ ${formatBytes(down)}`, extra: `↑ ${formatBytes(up)}` },
    { label: "Активность", value: activity, text: true }
  ];

  return (
    <Card className={cn(className)}>
      <CardContent className="@container grid gap-5">
        <div className="flex flex-wrap items-center gap-3">
          <UserAvatar email={user.email} size="lg" />
          <div className="grid min-w-0 flex-1 gap-1">
            <h2 className="truncate text-lg font-semibold" title={user.email}>
              {user.email}
            </h2>
            <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
              <StatusBadge status={user.status} kind="user" />
              <span>
                {user.role === "admin" ? "Администратор" : "Пользователь"}
                {isSelf && " (вы)"} · с {formatDate(user.created_at)}
              </span>
              {!user.email_verified && <span className="text-warning-foreground">почта не подтверждена</span>}
            </div>
          </div>
          <div className="flex gap-2 max-sm:w-full max-sm:[&>*]:flex-1">
            {user.status === "pending" && (
              <Button size="lg" onClick={onApprove}>
                Подтвердить
              </Button>
            )}
            {!isSelf && user.status !== "suspended" && (
              <Button variant="destructive" size="lg" onClick={onSuspend}>
                {user.status === "pending" ? "Отклонить" : "Заблокировать"}
              </Button>
            )}
            {user.status === "suspended" && (
              <Button size="lg" onClick={onRestore}>
                Восстановить
              </Button>
            )}
          </div>
        </div>

        <dl className="grid grid-cols-2 overflow-hidden rounded-lg border bg-muted/40 @2xl:grid-cols-4">
          {stats.map((stat, index) => (
            <div
              key={stat.label}
              className={
                "grid min-w-0 gap-0.5 px-3.5 py-2.5 " +
                (index % 2 === 1 ? "border-l " : "") +
                (index >= 2 ? "border-t @2xl:border-t-0 @2xl:border-l " : "")
              }
            >
              <dt className="text-xs text-muted-foreground">{stat.label}</dt>
              <dd
                className={
                  stat.text
                    ? "truncate text-sm leading-7 font-medium"
                    : "flex flex-wrap items-baseline gap-x-1.5 text-lg font-semibold whitespace-nowrap tabular-nums"
                }
                title={stat.extra ? `${stat.value} ${stat.extra}` : String(stat.value)}
              >
                {stat.value}
                {stat.extra && <span className="text-sm font-normal text-muted-foreground">{stat.extra}</span>}
              </dd>
            </div>
          ))}
        </dl>

        <SubscriptionForm
          key={`${user.id}-${user.paid_until}`}
          user={user}
          plans={plans}
          onChange={onChangeSubscription}
        />

        <Separator />

        <TrafficChart load={loadTraffic} />

        <Separator />

        <div className="grid gap-5 sm:grid-cols-[200px_minmax(0,1fr)]">
          <LimitForm key={`${user.id}-${user.key_limit}`} user={user} onSave={onSaveLimit} />
          <div className="grid content-start gap-2">
            <Label>Новый ключ</Label>
            {user.status === "approved" && !user.subscription_active ? (
              <p className="text-sm leading-9 text-muted-foreground">Подписка закончилась — сначала продлите.</p>
            ) : user.status === "approved" ? (
              <CreateKeyForm key={user.id} placeholder="Название устройства" submitLabel="Добавить" onCreate={onCreateKey} />
            ) : (
              <p className="text-sm leading-9 text-muted-foreground">
                {user.status === "pending" ? "Сначала подтвердите аккаунт." : "Сначала восстановите аккаунт."}
              </p>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
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
    <form className="grid content-start gap-2" onSubmit={submit}>
      <Label htmlFor={`limit-${user.id}`}>Лимит ключей</Label>
      <div className="flex gap-2">
        <Input
          id={`limit-${user.id}`}
          className="w-20 max-sm:flex-1"
          type="number"
          inputMode="numeric"
          min={0}
          max={100}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
        <Button variant="outline" size="lg" disabled={busy || !valid || value === user.key_limit}>
          Сохранить
        </Button>
      </div>
    </form>
  );
}
