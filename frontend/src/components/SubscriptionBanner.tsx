import { CalendarClock, CalendarX } from "lucide-react";
import { User } from "@/api";
import { daysWord, subscriptionState } from "@/lib/subscription";
import { cn } from "@/lib/utils";
import { formatDate } from "@/utils";

/** Подписка глазами пользователя: срок, предупреждение о скором конце или о том, что ключи отключены. */
export function SubscriptionBanner({ user, className }: { user: User; className?: string }) {
  const state = subscriptionState(user);
  if (state.kind === "forever") return null;
  if (state.kind === "active")
    return (
      <p className={cn("text-sm text-muted-foreground", className)}>Подписка действует до {formatDate(state.until)}</p>
    );
  const expired = state.kind === "expired";
  const Icon = expired ? CalendarX : CalendarClock;
  return (
    <div
      role={expired ? "alert" : "status"}
      className={cn(
        "flex gap-3 rounded-lg p-3 text-sm",
        expired ? "bg-destructive/10 text-destructive" : "bg-warning/15 text-warning-foreground",
        className
      )}
    >
      <Icon className="mt-0.5 size-4 shrink-0" />
      <div className="grid gap-0.5">
        <strong className="font-medium">
          {expired
            ? `Подписка закончилась ${formatDate(state.until)}`
            : `Подписка заканчивается ${formatDate(state.until)} — осталось ${daysWord(state.daysLeft)}`}
        </strong>
        <span className={expired ? "text-destructive/90" : "text-foreground/80"}>
          {expired
            ? "Ключи отключены, но сохранены: после продления заработают с теми же ссылками. Чтобы продлить, напишите администратору."
            : "Чтобы ключи не отключились, продлите подписку у администратора."}
        </span>
      </div>
    </div>
  );
}
