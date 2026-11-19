import { CheckStatus, VpnStatus } from "../api";
import { formatTime } from "../utils";

const TEXT: Record<Exclude<CheckStatus, "unknown">, { title: string; hint: string }> = {
  ok: { title: "VPN работает", hint: "Всё в порядке" },
  warn: { title: "VPN работает с перебоями", hint: "Возможны медленная загрузка и обрывы — мы уже знаем и разбираемся" },
  fail: { title: "Сбой VPN", hint: "Подключение сейчас может не работать — мы уже знаем и чиним" }
};

/** Общий статус VPN для пользователя: итог и задержка, без подробностей о серверах. */
export function VpnStatusBar({ status }: { status: VpnStatus | null }) {
  if (!status || status.status === "unknown") return null;
  const text = TEXT[status.status];
  return (
    <section className={`vpn-status ${status.status}`} aria-live="polite">
      <span className="vpn-status-dot" />
      <div>
        <strong>{text.title}</strong>
        <span>{status.status === "ok" ? "Подключение проверяется каждую минуту" : text.hint}</span>
      </div>
      <dl>
        {status.latency_ms != null && (
          <div>
            <dt>Отклик</dt>
            <dd>{Math.round(status.latency_ms)} мс</dd>
          </div>
        )}
        {status.checked_at && (
          <div>
            <dt>Проверено</dt>
            <dd>{formatTime(status.checked_at)}</dd>
          </div>
        )}
      </dl>
    </section>
  );
}
