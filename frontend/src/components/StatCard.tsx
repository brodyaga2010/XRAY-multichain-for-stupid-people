import { LucideIcon } from "lucide-react";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const toneClasses = {
  neutral: "bg-muted text-muted-foreground",
  good: "bg-success/12 text-success",
  warn: "bg-warning/15 text-warning-foreground",
  danger: "bg-destructive/10 text-destructive"
};

export function StatCard({
  label,
  value,
  helper,
  icon: Icon,
  tone = "neutral"
}: {
  label: string;
  value: string | number;
  helper: string;
  icon: LucideIcon;
  tone?: keyof typeof toneClasses;
}) {
  return (
    <Card size="sm" className="flex-row items-center gap-3 px-4 py-3.5">
      <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg", toneClasses[tone])}>
        <Icon className="size-4.5" />
      </span>
      <div className="grid min-w-0 gap-0.5">
        <span className="truncate text-xs font-medium text-muted-foreground">{label}</span>
        <strong className="text-xl leading-tight font-semibold tabular-nums">{value}</strong>
        <span className="truncate text-xs text-muted-foreground max-md:hidden">{helper}</span>
      </div>
    </Card>
  );
}
