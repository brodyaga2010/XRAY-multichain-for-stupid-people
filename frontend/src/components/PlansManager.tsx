import { FormEvent, useState } from "react";
import { Pencil, Star, Trash2 } from "lucide-react";
import { Plan, PlanInput } from "@/api";
import { useConfirm } from "@/components/ConfirmDialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { planDuration } from "@/lib/subscription";

const emptyPlan: PlanInput = { name: "", key_limit: 3, duration_days: 30, is_default: false };

/** Тарифы: список, создание, правка и удаление. */
export function PlansManager({
  plans,
  onSave,
  onDelete
}: {
  plans: Plan[];
  onSave: (plan: PlanInput, id?: number) => Promise<void>;
  onDelete: (plan: Plan) => Promise<void>;
}) {
  const [editing, setEditing] = useState<number | "new" | null>(plans.length === 0 ? "new" : null);
  const confirm = useConfirm();

  async function remove(plan: Plan) {
    const ok = await confirm({
      title: `Удалить тариф «${plan.name}»?`,
      description: "Пользователей на нём нет, удаление ничего не изменит у них.",
      action: "Удалить",
      destructive: true
    });
    if (ok) await onDelete(plan);
  }

  return (
    <div className="grid gap-4">
      <p className="text-sm text-muted-foreground">
        Тариф задаёт лимит ключей и срок подписки. Тариф со звёздочкой назначается при подтверждении заявки. Когда срок
        заканчивается, ключи отключаются; после продления снова работают с теми же ссылками.
      </p>
      {plans.length > 0 && (
        <ul className="grid divide-y rounded-lg border">
          {plans.map((plan) =>
            editing === plan.id ? (
              <li key={plan.id} className="p-3">
                <PlanForm
                  initial={plan}
                  submitLabel="Сохранить"
                  onCancel={() => setEditing(null)}
                  onSubmit={async (value) => {
                    await onSave(value, plan.id);
                    setEditing(null);
                  }}
                />
              </li>
            ) : (
              <li key={plan.id} className="flex items-center gap-3 px-3 py-2.5">
                <div className="grid min-w-0 flex-1 gap-0.5">
                  <span className="flex items-center gap-1.5 font-medium">
                    <span className="truncate">{plan.name}</span>
                    {plan.is_default && (
                      <Star className="size-3.5 shrink-0 fill-warning text-warning" aria-label="по умолчанию" />
                    )}
                  </span>
                  <span className="text-xs text-muted-foreground tabular-nums">
                    ключей: {plan.key_limit} · {planDuration(plan.duration_days)} · пользователей: {plan.users}
                  </span>
                </div>
                <Button variant="ghost" size="icon-sm" aria-label={`Изменить ${plan.name}`} onClick={() => setEditing(plan.id)}>
                  <Pencil />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                  disabled={plan.users > 0}
                  title={plan.users > 0 ? "Сначала переведите пользователей на другой тариф" : "Удалить"}
                  aria-label={`Удалить ${plan.name}`}
                  onClick={() => remove(plan)}
                >
                  <Trash2 />
                </Button>
              </li>
            )
          )}
        </ul>
      )}
      {editing === "new" ? (
        <div className="rounded-lg border p-3">
          <PlanForm
            initial={{ ...emptyPlan, is_default: plans.length === 0 }}
            submitLabel="Добавить тариф"
            onCancel={plans.length > 0 ? () => setEditing(null) : undefined}
            onSubmit={async (value) => {
              await onSave(value);
              setEditing(null);
            }}
          />
        </div>
      ) : (
        <Button variant="outline" className="justify-self-start" onClick={() => setEditing("new")}>
          Новый тариф
        </Button>
      )}
    </div>
  );
}

function PlanForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel
}: {
  initial: PlanInput;
  submitLabel: string;
  onSubmit: (plan: PlanInput) => Promise<void>;
  onCancel?: () => void;
}) {
  const [name, setName] = useState(initial.name);
  const [keys, setKeys] = useState(String(initial.key_limit));
  const [days, setDays] = useState(String(initial.duration_days));
  const [isDefault, setIsDefault] = useState(initial.is_default);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const keyLimit = Number(keys);
  const duration = Number(days);
  const valid =
    name.trim().length > 0 &&
    Number.isInteger(keyLimit) &&
    keyLimit >= 0 &&
    keyLimit <= 100 &&
    Number.isInteger(duration) &&
    duration >= 0 &&
    duration <= 3660;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!valid) return;
    setBusy(true);
    setError("");
    try {
      await onSubmit({ name: name.trim(), key_limit: keyLimit, duration_days: duration, is_default: isDefault });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось сохранить тариф");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="grid gap-3" onSubmit={submit}>
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_96px_120px]">
        <div className="grid gap-1.5">
          <Label htmlFor="plan-name">Название</Label>
          <Input id="plan-name" value={name} maxLength={64} onChange={(event) => setName(event.target.value)} autoFocus />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="plan-keys">Ключей</Label>
          <Input id="plan-keys" type="number" inputMode="numeric" min={0} max={100} value={keys} onChange={(event) => setKeys(event.target.value)} />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="plan-days">Дней (0 — ∞)</Label>
          <Input id="plan-days" type="number" inputMode="numeric" min={0} max={3660} value={days} onChange={(event) => setDays(event.target.value)} />
        </div>
      </div>
      <label className="flex items-center gap-2 text-sm">
        <Checkbox checked={isDefault} onCheckedChange={(value) => setIsDefault(value === true)} />
        Назначать новым пользователям
      </label>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <div className="flex gap-2">
        <Button disabled={busy || !valid}>{submitLabel}</Button>
        {onCancel && (
          <Button type="button" variant="ghost" onClick={onCancel}>
            Отмена
          </Button>
        )}
      </div>
    </form>
  );
}
