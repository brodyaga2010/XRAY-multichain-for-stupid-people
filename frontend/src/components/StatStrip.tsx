import { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

const toneClasses = {
  neutral: "text-muted-foreground",
  good: "text-success",
  warn: "text-warning-foreground",
  danger: "text-destructive"
};

export type Stat = {
  label: string;
  value: string | number;
  icon: LucideIcon;
  tone?: keyof typeof toneClasses;
  /** Куда ведёт нажатие; без него показатель не кликабельный. */
  onClick?: () => void;
  /** Пояснение при наведении — что именно посчитано. */
  hint?: string;
};

/** Показатели одной полосой: на ПК — в ряд, на телефоне — сеткой по две. */
export function StatStrip({ items }: { items: Stat[] }) {
  return (
    <div className="grid grid-cols-2 gap-px overflow-hidden rounded-xl bg-border ring-1 ring-foreground/10 sm:grid-cols-3 lg:grid-cols-6">
      {items.map(({ label, value, icon: Icon, tone = "neutral", onClick, hint }) => {
        const content = (
          <>
            <span className="flex min-w-0 items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <Icon className={cn("size-3.5 shrink-0", toneClasses[tone])} />
              <span className="truncate">{label}</span>
            </span>
            <strong className="text-xl leading-tight font-semibold tabular-nums">{value}</strong>
          </>
        );
        const cell = "grid min-w-0 content-start gap-1 bg-card px-3.5 py-3 text-left";
        return onClick ? (
          <button key={label} type="button" onClick={onClick} title={hint} className={cn(cell, "transition-colors hover:bg-muted/60")}>
            {content}
          </button>
        ) : (
          <div key={label} title={hint} className={cell}>
            {content}
          </div>
        );
      })}
    </div>
  );
}
