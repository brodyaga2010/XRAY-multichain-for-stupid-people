import { useEffect, useMemo, useState } from "react";
import { TrafficDay } from "@/api";
import { FilterChips } from "@/components/FilterChips";
import { useAutoRefresh } from "@/hooks";
import { formatDay, trafficBars, TrafficBar } from "@/lib/traffic";
import { cn } from "@/lib/utils";
import { formatBytes } from "@/utils";

type Period = "7" | "30" | "90";

const periods: Array<{ id: Period; label: string }> = [
  { id: "7", label: "7 дней" },
  { id: "30", label: "30 дней" },
  { id: "90", label: "90 дней" }
];

/** Трафик по дням: столбик — сумма скачанного и отданного; при наведении — подробности дня. */
export function TrafficChart({
  load,
  title = "Трафик по дням",
  className
}: {
  /** Загрузка истории за days дней; меняется — график перезагружается. */
  load: (days: number) => Promise<TrafficDay[]>;
  title?: string;
  className?: string;
}) {
  const [period, setPeriod] = useState<Period>("30");
  const [days, setDays] = useState<TrafficDay[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [hover, setHover] = useState<TrafficBar | null>(null);

  useEffect(() => {
    let alive = true;
    setFailed(false);
    load(Number(period))
      .then((loaded) => alive && setDays(loaded))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [load, period]);

  useAutoRefresh(() => load(Number(period)).then(setDays));

  const { bars, total } = useMemo(() => trafficBars(days ?? []), [days]);

  return (
    <div className={cn("grid gap-2", className)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{title}</h3>
        <FilterChips label="Период" options={periods} value={period} onChange={setPeriod} compact className="w-auto" />
      </div>
      <div className="flex justify-between gap-3 text-xs text-muted-foreground tabular-nums">
        <span className={cn(hover && "font-medium text-foreground")}>
          {hover ? formatDay(hover.day) : `Всего ${formatBytes(total)}`}
        </span>
        {hover && (
          <span className="font-medium text-foreground">
            {formatBytes(hover.total)} · ↓ {formatBytes(hover.down)} · ↑ {formatBytes(hover.up)}
          </span>
        )}
      </div>
      {failed ? (
        <p className="py-6 text-center text-sm text-muted-foreground">Не удалось загрузить историю</p>
      ) : (
        <div
          className="flex h-24 items-stretch gap-0.5 rounded-lg bg-muted/50 px-1 pt-2"
          onPointerLeave={() => setHover(null)}
          aria-hidden
        >
          {bars.map((bar) => (
            <div
              key={bar.day}
              className="group flex min-w-0 flex-1 cursor-default items-end"
              onPointerEnter={() => setHover(bar)}
            >
              <div
                className={cn(
                  "w-full rounded-t-[4px] bg-primary/70 transition-colors group-hover:bg-primary",
                  hover && hover.day !== bar.day && "bg-primary/40"
                )}
                style={{ height: `${bar.height}%` }}
              />
            </div>
          ))}
        </div>
      )}
      {bars.length > 0 && !failed && (
        <div className="flex justify-between text-xs text-muted-foreground tabular-nums" aria-hidden>
          <span>{formatDay(bars[0].day)}</span>
          <span>{formatDay(bars[bars.length - 1].day)}</span>
        </div>
      )}
      {/* Для скринридеров — те же данные таблицей. sr-only — на обёртке: таблица сама по высоте
          не сжимается и растягивала бы прокрутку, когда график стоит внутри таблицы ключей. */}
      <div className="sr-only">
        <table>
          <caption>{title}</caption>
          <thead>
            <tr>
              <th>День</th>
              <th>Скачано</th>
              <th>Отдано</th>
            </tr>
          </thead>
          <tbody>
            {bars
              .filter((bar) => bar.total > 0)
              .map((bar) => (
                <tr key={bar.day}>
                  <td>{formatDay(bar.day)}</td>
                  <td>{formatBytes(bar.down)}</td>
                  <td>{formatBytes(bar.up)}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
