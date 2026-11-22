import type { User } from "@/api";

const DAY_MS = 24 * 60 * 60 * 1000;

/** За сколько дней до конца срока предупреждать. */
export const SOON_DAYS = 3;

export type SubscriptionState =
  | { kind: "forever" }
  | { kind: "active"; until: string; daysLeft: number }
  | { kind: "soon"; until: string; daysLeft: number }
  | { kind: "expired"; until: string };

/** Состояние подписки: бессрочная, действует, скоро закончится или закончилась. */
export function subscriptionState(user: Pick<User, "paid_until" | "subscription_active">, now = Date.now()): SubscriptionState {
  if (!user.paid_until) return { kind: "forever" };
  const left = new Date(user.paid_until).getTime() - now;
  if (!user.subscription_active || left <= 0) return { kind: "expired", until: user.paid_until };
  const daysLeft = Math.ceil(left / DAY_MS);
  return { kind: daysLeft <= SOON_DAYS ? "soon" : "active", until: user.paid_until, daysLeft };
}

/** Значение для сортировки: бессрочные — в конце, закончившиеся — раньше всех по дате. */
export function paidUntilOrder(user: Pick<User, "paid_until">) {
  return user.paid_until ? new Date(user.paid_until).getTime() : Number.MAX_SAFE_INTEGER;
}

/** «2026-12-20» для поля даты из момента paid_until — по местному времени браузера. */
export function toDateInput(iso: string | null) {
  if (!iso) return "";
  const date = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** «3 дня», «1 день», «5 дней». */
export function daysWord(n: number) {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return `${n} день`;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return `${n} дня`;
  return `${n} дней`;
}

/** Длительность тарифа: «бессрочно», «30 дней». */
export function planDuration(days: number) {
  return days === 0 ? "бессрочно" : daysWord(days);
}
