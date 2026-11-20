import { FormEvent, useState } from "react";
import { Plan, SubscriptionChange, User } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/native-select";
import { daysWord, planDuration, subscriptionState, toDateInput } from "@/lib/subscription";
import { cn } from "@/lib/utils";
import { formatDate } from "@/utils";

const extendOptions = [1, 3, 12];

/** Состояние подписки: «до 20.12.2026 · осталось 30 дней»; short — без остатка дней (кроме последних). */
export function SubscriptionSummary({ user, short = false, className }: { user: User; short?: boolean; className?: string }) {
  const state = subscriptionState(user);
  if (state.kind === "forever") return <span className={cn("text-muted-foreground", className)}>бессрочно</span>;
  if (state.kind === "expired")
    return (
      <span className={cn("font-medium text-destructive", className)}>
        {short ? "истекла" : "закончилась"} {formatDate(state.until)}
      </span>
    );
  const showLeft = !short || state.kind === "soon";
  return (
    <span className={cn(state.kind === "soon" && "font-medium text-warning-foreground", className)}>
      до {formatDate(state.until)}
      {showLeft && <span className="font-normal text-muted-foreground"> · {short ? daysWord(state.daysLeft) : `осталось ${daysWord(state.daysLeft)}`}</span>}
    </span>
  );
}

/** Подписка пользователя в карточке администратора: тариф, продление, точная дата. */
export function SubscriptionForm({
  user,
  plans,
  onChange
}: {
  user: User;
  plans: Plan[];
  onChange: (change: SubscriptionChange) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [date, setDate] = useState(toDateInput(user.paid_until));
  const currentPlan = plans.find((plan) => plan.id === user.plan_id) ?? null;
  const state = subscriptionState(user);

  async function run(change: SubscriptionChange) {
    setBusy(true);
    try {
      await onChange(change);
    } finally {
      setBusy(false);
    }
  }

  /** Смена тарифа; если срока нет или он прошёл — начинается новый срок тарифа с сегодняшнего дня. */
  function changePlan(value: string) {
    const plan = plans.find((item) => String(item.id) === value) ?? null;
    const change: SubscriptionChange = { plan_id: plan?.id ?? null };
    if (plan && plan.duration_days > 0 && (state.kind === "forever" || state.kind === "expired")) {
      const until = new Date();
      until.setDate(until.getDate() + plan.duration_days);
      change.paid_until = toDateInput(until.toISOString());
    }
    void run(change);
  }

  function saveDate(event: FormEvent) {
    event.preventDefault();
    if (date && date !== toDateInput(user.paid_until)) void run({ paid_until: date });
  }

  return (
    <section className="grid gap-3" aria-label="Подписка">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h3 className="text-sm font-medium">Подписка</h3>
        <SubscriptionSummary user={user} className="text-sm" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid content-start gap-2">
          <Label htmlFor={`plan-${user.id}`}>Тариф</Label>
          <NativeSelect
            id={`plan-${user.id}`}
            value={user.plan_id ?? ""}
            disabled={busy}
            onChange={(event) => changePlan(event.target.value)}
          >
            <option value="">Без тарифа</option>
            {plans.map((plan) => (
              <option key={plan.id} value={plan.id}>
                {plan.name}
              </option>
            ))}
          </NativeSelect>
          {currentPlan && (
            <p className="text-xs text-muted-foreground">
              Ключей: {currentPlan.key_limit} · срок: {planDuration(currentPlan.duration_days)}
            </p>
          )}
        </div>
        <div className="grid content-start gap-2">
          <Label>Продлить</Label>
          <div className="flex gap-2 [&>*]:flex-1">
            {extendOptions.map((months) => (
              <Button key={months} variant="outline" size="lg" disabled={busy} onClick={() => run({ extend_months: months })}>
                +{months} мес
              </Button>
            ))}
          </div>
        </div>
        <form className="grid content-start gap-2 sm:col-span-2" onSubmit={saveDate}>
          <Label htmlFor={`paid-${user.id}`}>Оплачено до (включительно)</Label>
          <div className="flex flex-wrap gap-2">
            <Input
              id={`paid-${user.id}`}
              type="date"
              className="min-w-40 flex-1 sm:w-44 sm:flex-initial"
              value={date}
              onChange={(event) => setDate(event.target.value)}
            />
            <Button variant="outline" size="lg" disabled={busy || !date || date === toDateInput(user.paid_until)}>
              Сохранить
            </Button>
            {user.paid_until && (
              <Button type="button" variant="ghost" size="lg" disabled={busy} onClick={() => run({ paid_until: null })}>
                Сделать бессрочной
              </Button>
            )}
          </div>
        </form>
      </div>
    </section>
  );
}
