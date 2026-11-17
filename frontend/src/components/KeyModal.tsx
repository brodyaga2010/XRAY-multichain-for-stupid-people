import { QrCode, X } from "lucide-react";
import { useEffect, useState } from "react";
import { VpnKey } from "../api";
import { CopyButton } from "./CopyButton";
import { QrImage } from "./QrImage";

const transportLabels: Record<string, string> = {
  tcp: "Основное (TCP)",
  grpc: "Запасное (gRPC)",
  xhttp: "Запасное (XHTTP)"
};

export function KeyModal({ keyItem, onClose }: { keyItem: VpnKey; onClose: () => void }) {
  const [qrFor, setQrFor] = useState<string | null>(null);
  const disabled = keyItem.status === "suspended";

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
        <div className="panel-heading">
          <h2>Подключение: {keyItem.name}</h2>
          <button className="icon-button" onClick={onClose} title="Закрыть">
            <X size={16} />
          </button>
        </div>
        {disabled ? (
          <p className="error">Ключ отключён администратором</p>
        ) : (
          <>
            <div className="link-list">
              {keyItem.links.map((link) => (
                <div className="link-row" key={link.transport}>
                  <strong>{transportLabels[link.transport] ?? link.label}</strong>
                  <code className="link-uri">{link.uri}</code>
                  <div className="link-actions">
                    <CopyButton text={link.uri} />
                    <button
                      type="button"
                      className="small ghost"
                      onClick={() => setQrFor(qrFor === link.uri ? null : link.uri)}
                    >
                      <QrCode size={15} />
                      <span>QR-код</span>
                    </button>
                  </div>
                  {qrFor === link.uri && <QrImage value={link.uri} />}
                </div>
              ))}
              {keyItem.links.length === 0 && <div className="empty">Ссылок нет</div>}
            </div>
            <div className="instructions">
              <h3>Как подключиться</h3>
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
          </>
        )}
      </div>
    </div>
  );
}
