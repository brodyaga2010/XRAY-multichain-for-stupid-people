import { useMemo, useState } from "react";
import { CheckStatus, MonitorPoint, MonitorStatus } from "../api";
import { formatDateTime, formatDuration, formatTime } from "../utils";

const DAY_MS = 24 * 60 * 60 * 1000;
const MAX_INCIDENTS = 5;

const STATUS_TEXT: Record<CheckStatus, string> = {
  ok: "в норме",
  warn: "с перебоями",
  fail: "не работает",
  unknown: "нет данных"
};

function overall(status: MonitorStatus): { tone: CheckStatus; text: string } {
  const statuses = status.checks.map((check) => check.status);
  if (!status.checked_at) return { tone: "unknown", text: "Первая проверка ещё идёт" };
  if (statuses.includes("fail")) return { tone: "fail", text: "Есть сбой" };
  if (statuses.includes("warn")) return { tone: "warn", text: "Работает с перебоями" };
  return { tone: "ok", text: "Всё работает" };
}

function seconds(ms: number) {
  return `${(ms / 1000).toFixed(2).replace(".", ",")} с`;
}

/** Задержка цепочки за сутки: линия по удачным проверкам, красные полосы — проверки без ответа. */
function LatencyChart({ points, now }: { points: MonitorPoint[]; now: number }) {
  const [hover, setHover] = useState<MonitorPoint | null>(null);
  const width = 600;
  const height = 120;
  const from = now - DAY_MS;

  const { yMax, segments, failures, parsed } = useMemo(() => {
    const parsed = points
      .map((point) => ({ point, t: new Date(point.at).getTime() }))
      .filter(({ t }) => t >= from);
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

  function onMove(event: React.PointerEvent<SVGSVGElement>) {
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
    <div className="latency-chart">
      <div className="latency-chart-head">
        <span>Задержка цепочки за сутки</span>
        <span className="latency-chart-readout">
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
        onPointerMove={onMove}
        onPointerLeave={() => setHover(null)}
      >
        {[0.25, 0.5, 0.75].map((share) => (
          <line key={share} className="grid" x1={0} x2={width} y1={height * share} y2={height * share} />
        ))}
        {failures.map((fx, index) => (
          <rect key={index} className="failure" x={fx - 2} width={4} y={0} height={height} />
        ))}
        {segments.map((segment, index) => (
          <polyline key={index} className="line" points={segment} />
        ))}
      </svg>
      <div className="latency-chart-axis">
        <span>{formatTime(new Date(from))}</span>
        <span>{formatTime(new Date(from + DAY_MS / 2))}</span>
        <span>сейчас</span>
      </div>
      {parsed.length === 0 && <div className="latency-chart-empty">Данных пока нет — они копятся с запуска панели</div>}
    </div>
  );
}

export function HealthPanel({ status }: { status: MonitorStatus | null }) {
  if (!status) return null;
  if (!status.enabled) {
    return (
      <section className="table-panel health-panel animate-in">
        <div className="panel-heading">
          <h2>Состояние VPN</h2>
          <span>мониторинг выключен</span>
        </div>
      </section>
    );
  }
  const summary = overall(status);
  const now = status.checked_at ? new Date(status.checked_at).getTime() : Date.now();
  const incidents = status.incidents.slice(0, MAX_INCIDENTS);

  return (
    <section className="table-panel health-panel animate-in">
      <div className="panel-heading">
        <h2>Состояние VPN</h2>
        <div className="panel-heading-side">
          <span className={`health-badge ${summary.tone}`}>{summary.text}</span>
          {status.checked_at && <span>проверено в {formatTime(status.checked_at)}</span>}
        </div>
      </div>
      <div className="health-body">
        <ul className="health-checks">
          {status.checks.map((check) => (
            <li key={check.id} className={`health-check ${check.status}`}>
              <span className="health-dot" aria-label={STATUS_TEXT[check.status]} title={STATUS_TEXT[check.status]} />
              <div>
                <strong>{check.name}</strong>
                <span>{check.value || STATUS_TEXT[check.status]}</span>
                {check.detail && check.status !== "ok" && <em>{check.detail}</em>}
              </div>
            </li>
          ))}
        </ul>
        <div className="health-side">
          <LatencyChart points={status.history} now={now} />
          <div className="health-incidents">
            <h3>Последние сбои</h3>
            {incidents.length === 0 ? (
              <div className="empty">Сбоев не было</div>
            ) : (
              <ul>
                {incidents.map((incident) => {
                  const start = new Date(incident.started_at).getTime();
                  const end = incident.ended_at ? new Date(incident.ended_at).getTime() : null;
                  return (
                    <li key={`${incident.check}-${incident.started_at}`} className={incident.status}>
                      <span className="health-dot" />
                      <div>
                        <strong>{incident.name}</strong>
                        <span>
                          {formatDateTime(incident.started_at)}
                          {end ? ` – ${formatTime(incident.ended_at!)} · ${formatDuration(end - start)}` : " · идёт сейчас"}
                        </span>
                        {incident.detail && <em>{incident.detail}</em>}
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
