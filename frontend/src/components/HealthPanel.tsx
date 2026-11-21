import { useMemo, useState } from "react";
import { CheckStatus, MonitorPoint, MonitorStatus } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DAY_MS, formatLatency, LatencyBar, latencyBars } from "@/lib/latency";
import { cn } from "@/lib/utils";
import { formatDateTime, formatDuration, formatTime } from "@/utils";

const MAX_INCIDENTS = 5;

const STATUS_TEXT: Record<CheckStatus, string> = {
  ok: "в норме",
  warn: "с перебоями",
  fail: "не работает",
  unknown: "нет данных"
};

/** Что измеряет проверка: у цепочки и канала обе цифры — задержка, но разных участков. */
const CHECK_HINT: Record<string, string> = {
  chain: "Отклик: запрос к сайту через оба сервера, как у пользователей",
  exit: "Только участок между входным и выходным сервером — часть отклика"
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

function Dot({ status }: { status: CheckStatus }) {
  return <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", dotClass[status])} />;
}

/** Отклик (проверка цепочки) за сутки по получасам — как график трафика; красный столбик — были проверки без ответа. */
function LatencyChart({ points, now }: { points: MonitorPoint[]; now: number }) {
  const [hover, setHover] = useState<LatencyBar | null>(null);
  const { bars, from, scale, avg } = useMemo(() => latencyBars(points, now), [points, now]);
  const empty = bars.every((bar) => bar.checks === 0);

  function details(bar: LatencyBar) {
    if (bar.checks === 0) return "нет данных";
    const parts: string[] = [];
    if (bar.avg != null) parts.push(`в среднем ${formatLatency(bar.avg)}`);
    if (bar.peak != null && bar.peak - (bar.avg ?? 0) >= 10) parts.push(`пик ${formatLatency(bar.peak)}`);
    if (bar.failed) parts.push(`без ответа ${bar.failed} из ${bar.checks}`);
    return parts.join(" · ");
  }

  return (
    <div className="grid gap-2">
      <h3 className="text-sm font-medium">Отклик за сутки</h3>
      <div className="flex flex-wrap justify-between gap-x-3 text-xs text-muted-foreground tabular-nums">
        <span className={cn(hover && "font-medium text-foreground")}>
          {hover
            ? `${formatTime(new Date(hover.start))}–${formatTime(new Date(hover.end))}`
            : avg != null
              ? `В среднем ${formatLatency(avg)}`
              : "Данных пока нет"}
        </span>
        <span className={cn(hover && "font-medium text-foreground", hover?.failed && "text-destructive")}>
          {hover ? details(hover) : `шкала до ${formatLatency(scale)}`}
        </span>
      </div>
      <div className="relative">
        <div
          className="flex h-24 items-stretch gap-0.5 rounded-lg bg-muted/50 px-1 pt-2"
          onPointerLeave={() => setHover(null)}
          aria-hidden
        >
          {bars.map((bar) => (
            <div key={bar.start} className="group flex min-w-0 flex-1 cursor-default items-end" onPointerEnter={() => setHover(bar)}>
              <div
                className={cn(
                  "w-full rounded-t-[4px] transition-colors",
                  bar.failed ? "bg-destructive/70 group-hover:bg-destructive" : "bg-primary/70 group-hover:bg-primary",
                  hover && hover.start !== bar.start && (bar.failed ? "bg-destructive/40" : "bg-primary/40"),
                  bar.checks === 0 && "h-0.5 rounded-none bg-muted-foreground/15 group-hover:bg-muted-foreground/30"
                )}
                style={bar.checks ? { height: `${bar.height}%` } : undefined}
              />
            </div>
          ))}
        </div>
        {empty && (
          <div className="absolute inset-0 grid place-items-center px-4 text-center text-sm text-muted-foreground">
            Данных пока нет — они копятся с запуска кабинета
          </div>
        )}
      </div>
      <div className="flex justify-between text-xs text-muted-foreground tabular-nums" aria-hidden>
        <span>{formatTime(new Date(from))}</span>
        <span>{formatTime(new Date(from + DAY_MS / 2))}</span>
        <span>сейчас</span>
      </div>
      {/* Для скринридеров — те же данные таблицей. */}
      <table className="sr-only">
        <caption>Отклик за сутки, по получасам</caption>
        <thead>
          <tr>
            <th>Время</th>
            <th>Задержка</th>
          </tr>
        </thead>
        <tbody>
          {bars
            .filter((bar) => bar.checks > 0)
            .map((bar) => (
              <tr key={bar.start}>
                <td>
                  {formatTime(new Date(bar.start))}–{formatTime(new Date(bar.end))}
                </td>
                <td>{details(bar)}</td>
              </tr>
            ))}
        </tbody>
      </table>
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
        <CardTitle>Проверки</CardTitle>
        {status.checked_at && (
          <CardDescription className="tabular-nums">Проверено в {formatTime(status.checked_at)}</CardDescription>
        )}
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
                {/* Длинное значение на узком экране уходит под название, а не обрезает его. */}
                <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                  <span className="text-sm font-medium">{check.name}</span>
                  <span className="text-sm text-muted-foreground tabular-nums">{check.value || STATUS_TEXT[check.status]}</span>
                </div>
                {CHECK_HINT[check.id] && <span className="text-xs text-muted-foreground">{CHECK_HINT[check.id]}</span>}
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
