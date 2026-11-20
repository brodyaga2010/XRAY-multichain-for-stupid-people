import { CheckStatus, VpnStatus } from "@/api";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { formatTime } from "@/utils";

const TEXT: Record<Exclude<CheckStatus, "unknown">, { short: string; title: string; hint: string }> = {
  ok: { short: "", title: "VPN работает", hint: "Подключение проверяется каждую минуту." },
  warn: {
    short: "Перебои",
    title: "VPN работает с перебоями",
    hint: "Возможны медленная загрузка и обрывы. Мы уже знаем и разбираемся."
  },
  fail: { short: "Сбой", title: "Сбой VPN", hint: "Подключение сейчас может не работать. Мы уже знаем и чиним." }
};

const dot: Record<Exclude<CheckStatus, "unknown">, string> = {
  ok: "bg-success ring-success/20",
  warn: "bg-warning ring-warning/25",
  fail: "bg-destructive ring-destructive/20"
};

const pill: Record<Exclude<CheckStatus, "unknown">, string> = {
  ok: "",
  warn: "border-warning/40 bg-warning/10 text-warning-foreground hover:bg-warning/15",
  fail: "border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/15"
};

/** Компактный статус VPN в шапке: точка и отклик, по нажатию — подробности. */
export function VpnStatusPill({ status, onDetails }: { status: VpnStatus | null; onDetails?: () => void }) {
  if (!status || status.status === "unknown") return null;
  const tone = status.status;
  const text = TEXT[tone];
  const latency = status.latency_ms != null ? `${Math.round(status.latency_ms)} мс` : null;

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn("h-9 rounded-full px-3 tabular-nums", pill[tone])}
          aria-label={`${text.title}${latency ? `, отклик ${latency}` : ""}`}
        >
          <span className={cn("size-2 rounded-full ring-3", dot[tone])} />
          {text.short || latency || "VPN"}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72">
        <div className="grid gap-3">
          <div className="flex items-center gap-2 font-medium">
            <span className={cn("size-2 rounded-full ring-3", dot[tone])} />
            {text.title}
          </div>
          <p className="text-sm text-muted-foreground">{text.hint}</p>
          <dl className="flex gap-6 text-sm">
            {latency && (
              <div>
                <dt className="text-xs text-muted-foreground">Отклик</dt>
                <dd className="font-semibold tabular-nums">{latency}</dd>
              </div>
            )}
            {status.checked_at && (
              <div>
                <dt className="text-xs text-muted-foreground">Проверено</dt>
                <dd className="font-semibold tabular-nums">{formatTime(status.checked_at)}</dd>
              </div>
            )}
          </dl>
          {onDetails && (
            <Button variant="link" className="h-auto justify-self-start p-0" onClick={onDetails}>
              Подробнее о состоянии
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
