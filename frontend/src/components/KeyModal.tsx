import { ChevronDown, QrCode, X } from "lucide-react";
import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { KeyLink, VpnKey } from "../api";
import { useMediaQuery } from "../hooks";
import { keyTitle } from "../utils";
import { CopyButton } from "./CopyButton";
import { QrImage } from "./QrImage";

const transportLabels: Record<string, string> = {
  tcp: "Основное (TCP)",
  grpc: "Запасное (gRPC)",
  xhttp: "Запасное (XHTTP)"
};

function LinkBlock({ link, primary, alwaysQr }: { link: KeyLink; primary: boolean; alwaysQr: boolean }) {
  const [qrOpen, setQrOpen] = useState(false);
  const qrShown = alwaysQr || qrOpen;

  return (
    <section className={`link-block ${primary ? "primary-block" : ""}`}>
      <h3>{transportLabels[link.transport] ?? link.label}</h3>
      <code className="link-line">{link.uri}</code>
      <div className="link-buttons">
        <CopyButton text={link.uri} label="Скопировать ключ" className={primary ? "primary copy-main" : "ghost"} />
        {!alwaysQr && (
          <button type="button" className="ghost" onClick={() => setQrOpen((current) => !current)}>
            <QrCode size={16} />
            <span>{qrOpen ? "Скрыть QR" : "Показать QR"}</span>
          </button>
        )}
      </div>
      {qrShown && <QrImage value={link.uri} />}
    </section>
  );
}

export function KeyModal({ keyItem, onClose }: { keyItem: VpnKey; onClose: () => void }) {
  const isPhone = useMediaQuery("(max-width: 640px)");
  const disabled = keyItem.status === "suspended";
  const primaryLink = keyItem.links.find((link) => link.transport === "tcp") ?? keyItem.links[0];
  const fallbackLinks = keyItem.links.filter((link) => link !== primaryLink);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  return createPortal(
    <div
      className="modal-backdrop"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div className="modal" role="dialog" aria-modal="true" aria-label={`Подключение: ${keyTitle(keyItem)}`}>
        <header className="modal-head">
          <div className="modal-title">
            <span>Подключение</span>
            <h2 title={keyTitle(keyItem)}>{keyTitle(keyItem)}</h2>
          </div>
          <button className="icon-button" onClick={onClose} aria-label="Закрыть" title="Закрыть">
            <X size={20} />
          </button>
        </header>
        <div className="modal-body">
          {disabled ? (
            <p className="notice error-notice">Ключ отключён администратором</p>
          ) : (
            <>
              {primaryLink ? (
                <LinkBlock link={primaryLink} primary alwaysQr={!isPhone} />
              ) : (
                <div className="empty">Ссылок нет</div>
              )}
              {fallbackLinks.length > 0 && (
                <details className="fold">
                  <summary>
                    <span>Запасные варианты (gRPC, XHTTP)</span>
                    <ChevronDown size={18} />
                  </summary>
                  <div className="fold-body">
                    {fallbackLinks.map((link) => (
                      <LinkBlock key={link.transport} link={link} primary={false} alwaysQr={false} />
                    ))}
                  </div>
                </details>
              )}
              <details className="fold">
                <summary>
                  <span>Как подключиться</span>
                  <ChevronDown size={18} />
                </summary>
                <div className="fold-body instructions">
                  <ol>
                    <li>
                      Установите приложение. Android: v2rayNG, Hiddify. iPhone: Streisand, Hiddify, v2RayTun. Windows и
                      macOS: Hiddify, v2rayN. AmneziaVPN тоже подходит.
                    </li>
                    <li>Скопируйте ссылку «Основное (TCP)».</li>
                    <li>В приложении нажмите «Добавить из буфера» или «Вставить ключ».</li>
                    <li>Подключитесь.</li>
                  </ol>
                  <p>Если не работает — попробуйте запасные ссылки.</p>
                </div>
              </details>
            </>
          )}
        </div>
      </div>
    </div>,
    document.body
  );
}
