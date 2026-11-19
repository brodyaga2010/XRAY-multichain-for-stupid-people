import { useEffect, useRef, useState } from "react";
import { CheckStatus, VpnStatus } from "../api";
import { formatTime } from "../utils";

const TEXT: Record<Exclude<CheckStatus, "unknown">, { short: string; title: string; hint: string }> = {
  ok: { short: "", title: "VPN работает", hint: "Подключение проверяется каждую минуту." },
  warn: {
    short: "Перебои",
    title: "VPN работает с перебоями",
    hint: "Возможны медленная загрузка и обрывы. Мы уже знаем и разбираемся."
  },
  fail: { short: "Сбой", title: "Сбой VPN", hint: "Подключение сейчас может не работать. Мы уже знаем и чиним." }
};

/** Компактный статус VPN в шапке: точка и отклик, по нажатию — подробности. */
export function VpnStatusPill({ status, onDetails }: { status: VpnStatus | null; onDetails?: () => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointer = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => event.key === "Escape" && setOpen(false);
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  if (!status || status.status === "unknown") return null;
  const text = TEXT[status.status];
  const latency = status.latency_ms != null ? `${Math.round(status.latency_ms)} мс` : null;
  const label = text.short || latency || "VPN";

  return (
    <div className="vpn-pill-wrap" ref={ref}>
      <button
        type="button"
        className={`vpn-pill ${status.status}`}
        aria-expanded={open}
        aria-label={`${text.title}${latency ? `, отклик ${latency}` : ""}`}
        onClick={() => setOpen((current) => !current)}
      >
        <i />
        <span>{label}</span>
      </button>
      {open && (
        <div className={`vpn-popover ${status.status}`} role="dialog" aria-label="Состояние VPN">
          <strong>
            <i /> {text.title}
          </strong>
          <p>{text.hint}</p>
          <dl>
            {latency && (
              <div>
                <dt>Отклик</dt>
                <dd>{latency}</dd>
              </div>
            )}
            {status.checked_at && (
              <div>
                <dt>Проверено</dt>
                <dd>{formatTime(status.checked_at)}</dd>
              </div>
            )}
          </dl>
          {onDetails && (
            <button
              type="button"
              className="link-button"
              onClick={() => {
                setOpen(false);
                onDetails();
              }}
            >
              Подробнее на «Обзоре»
            </button>
          )}
        </div>
      )}
    </div>
  );
}
