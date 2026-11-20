import { PointerEvent as ReactPointerEvent, useMemo, useState } from "react";
import { CheckStatus, MonitorPoint, MonitorStatus } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { formatDateTime, formatDuration, formatTime } from "@/utils";

const DAY_MS = 24 * 60 * 60 * 1000;
const MAX_INCIDENTS = 5;

const STATUS_TEXT: Record<CheckStatus, string> = {
  ok: "в норме",
  warn: "с перебоями",
  fail: "не работает",
  unknown: "нет данных"
};

const dotClass: Record<CheckStatus, string> = {
  ok: "bg-success",
  warn: "bg-warning",
  fail: "bg-destructive",
  unknown: "bg-muted-foreground/40"
};

const badgeClass: Record<CheckStatus, string> = {
  ok: "bg-success/12 text-success",
  warn: "bg-warning/15 text-warning-foreground",
  fail: "bg-destructive/10 text-destructive",
  unknown: "bg-muted text-muted-foreground"
};

/** Общий итог проверок: tone для цвета, text — подпись. */
export function overall(status: MonitorStatus): { tone: CheckStatus; text: string } {
  const statuses = status.checks.map((check) => check.status);
  if (!status.checked_at) return { tone: "unknown", text: "Первая проверка ещё идёт" };
  if (statuses.includes("fail")) return { tone: "fail", text: "Есть сбой" };
  if (statuses.includes("warn")) return { tone: "warn", text: "Работает с перебоями" };
  return { tone: "ok", text: "Всё работает" };
}

function seconds(ms: number) {
  return `${(ms / 1000).toFixed(2).replace(".", ",")} с`;
}

function Dot({ status }: { status: CheckStatus }) {
  return <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", dotClass[status])} />;
}

/** Задержка цепочки за сутки: линия по удачным проверкам, красные полосы — проверки без ответа. */
function LatencyChart({ points, now }: { points: MonitorPoint[]; now: number }) {
  const [hover, setHover] = useState<MonitorPoint | null>(null);
  const width = 600;
  const height = 120;
  const from = now - DAY_MS;

  const { yMax, segments, failures, parsed } = useMemo(() => {
    const parsed = points.map((point) => ({ point, t: new Date(point.at).getTime() })).filter(({ t }) => t >= from);
    const values = parsed.flatMap(({ point }) => (point.ms != null ? [point.ms] : []));
    const peak = values.length ? Math.max(...values) : 0;
    const yMax = Math.max(1000, Math.ceil(peak / 500) * 500);
    const x = (t: number) => ((t - from) / DAY_MS) * width;
    const y = (ms: number) => height - (Math.min(ms, yMax) / yMax) * height;

    const segments: string[] = [];
    let current: string[] = [];
    for (const { point, t } of parsed) {
      if (point.ms == null || point.status === "fail") {
        if (current.length) segments.push(current.join(" "));
        current = [];
        continue;
      }
      current.push(`${x(t).toFixed(1)},${y(point.ms).toFixed(1)}`);
    }
    if (current.length) segments.push(current.join(" "));
    const failures = parsed.filter(({ point }) => point.status === "fail").map(({ t }) => x(t));
    return { yMax, segments, failures, parsed };
  }, [points, from]);

  function onMove(event: ReactPointerEvent<SVGSVGElement>) {
    if (!parsed.length) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const t = from + ((event.clientX - rect.left) / rect.width) * DAY_MS;
    let best = parsed[0];
    for (const item of parsed) {
      if (Math.abs(item.t - t) < Math.abs(best.t - t)) best = item;
    }
    setHover(Math.abs(best.t - t) < 15 * 60 * 1000 ? best.point : null);
  }

  return (
    <div className="relative grid gap-1.5">
      <div className="flex justify-between gap-3 text-xs text-muted-foreground tabular-nums">
        <span>Задержка цепочки за сутки</span>
        <span className={cn(hover && "font-medium text-foreground")}>
          {hover
            ? `${formatTime(hover.at)} · ${hover.ms != null && hover.status !== "fail" ? seconds(hover.ms) : "нет ответа"}`
            : `шкала до ${seconds(yMax)}`}
        </span>
      </div>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="none"
        role="img"
        aria-label="График задержки цепочки за 24 часа"
        className="block h-24 w-full touch-pan-y rounded-lg bg-muted/50"
        onPointerMove={onMove}
        onPointerLeave={() => setHover(null)}
      >
        {[0.25, 0.5, 0.75].map((share) => (
          <line
            key={share}
            x1={0}
            x2={width}
            y1={height * share}
            y2={height * share}
            className="stroke-border"
            strokeWidth={1}
            vectorEffect="non-scaling-stroke"
          />
        ))}
        {failures.map((fx, index) => (
          <rect key={index} x={fx - 2} width={4} y={0} height={height} className="fill-destructive/40" />
        ))}
        {segments.map((segment, index) => (
          <polyline
            key={index}
            points={segment}
            fill="none"
            className="stroke-primary"
            strokeWidth={1.5}
            strokeLinejoin="round"
            vectorEffect="non-scaling-stroke"
          />
        ))}
      </svg>
      <div className="flex justify-between text-xs text-muted-foreground tabular-nums">
        <span>{formatTime(new Date(from))}</span>
        <span>{formatTime(new Date(from + DAY_MS / 2))}</span>
        <span>сейчас</span>
      </div>
      {parsed.length === 0 && (
        <div className="absolute inset-x-0 top-6 bottom-6 grid place-items-center text-center text-sm text-muted-foreground">
          Данных пока нет — они копятся с запуска кабинета
        </div>
      )}
    </div>
  );
}

export function HealthPanel({ status }: { status: MonitorStatus | null }) {
  if (!status) return null;
  if (!status.enabled) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Состояние VPN</CardTitle>
          <CardDescription>Мониторинг выключен</CardDescription>
        </CardHeader>
      </Card>
    );
  }
  const summary = overall(status);
  const now = status.checked_at ? new Date(status.checked_at).getTime() : Date.now();
  const incidents = status.incidents.slice(0, MAX_INCIDENTS);

  return (
    <Card className="gap-4">
      <CardHeader>
        <CardTitle className="flex items-baseline gap-2">
          Проверки
          {status.checked_at && (
            <span className="text-xs font-normal text-muted-foreground tabular-nums">проверено в {formatTime(status.checked_at)}</span>
          )}
        </CardTitle>
        <CardAction>
          <Badge className={cn("border-0", badgeClass[summary.tone])}>{summary.text}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="grid items-start gap-6 lg:grid-cols-[5fr_7fr]">
        <ul className="grid divide-y">
          {status.checks.map((check) => (
            <li key={check.id} className="flex gap-2.5 py-2 first:pt-0 last:pb-0">
              <Dot status={check.status} />
              <div className="grid min-w-0 flex-1 gap-0.5">
                <div className="flex items-baseline justify-between gap-3">
                  <span className="truncate text-sm font-medium" title={check.name}>
                    {check.name}
                  </span>
                  <span className="shrink-0 text-sm text-muted-foreground tabular-nums">{check.value || STATUS_TEXT[check.status]}</span>
                </div>
                {check.detail && check.status !== "ok" && (
                  <span className={cn("text-xs break-words", check.status === "fail" ? "text-destructive" : "text-muted-foreground")}>
                    {check.detail}
                  </span>
                )}
              </div>
            </li>
          ))}
        </ul>
        <div className="grid gap-4">
          <LatencyChart points={status.history} now={now} />
          <div className="grid gap-2">
            <h3 className="text-sm font-medium">Последние сбои</h3>
            {incidents.length === 0 ? (
              <p className="text-sm text-muted-foreground">Сбоев не было</p>
            ) : (
              <ul className="grid divide-y">
                {incidents.map((incident) => {
                  const start = new Date(incident.started_at).getTime();
                  const end = incident.ended_at ? new Date(incident.ended_at).getTime() : null;
                  return (
                    <li key={`${incident.check}-${incident.started_at}`} className="flex gap-3 py-2 first:pt-0 last:pb-0">
                      <Dot status={incident.status} />
                      <div className="grid min-w-0 gap-0.5">
                        <span className="text-sm font-medium">{incident.name}</span>
                        <span className="text-xs text-muted-foreground tabular-nums">
                          {formatDateTime(incident.started_at)}
                          {end ? ` – ${formatTime(incident.ended_at!)} · ${formatDuration(end - start)}` : " · идёт сейчас"}
                        </span>
                        {incident.detail && <span className="text-xs break-words text-muted-foreground">{incident.detail}</span>}
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
