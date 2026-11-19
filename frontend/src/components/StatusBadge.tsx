import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

const labels: Record<string, string> = {
  active: "Активен",
  suspended: "Отключён",
  revoked: "Отозван",
  pending: "Ждёт подтверждения",
  approved: "Подтверждён"
};

const tones: Record<string, string> = {
  active: "bg-success/12 text-success",
  approved: "bg-success/12 text-success",
  pending: "bg-warning/15 text-warning-foreground",
  suspended: "bg-destructive/10 text-destructive",
  revoked: "bg-muted text-muted-foreground"
};

/** Статус ключа или пользователя цветной плашкой. */
export function StatusBadge({
  status,
  kind = "key",
  className
}: {
  status: string;
  kind?: "key" | "user";
  className?: string;
}) {
  const label = kind === "user" && status === "suspended" ? "Заблокирован" : labels[status] ?? status;
  return <Badge className={cn("border-0", tones[status] ?? "bg-muted text-muted-foreground", className)}>{label}</Badge>;
}
