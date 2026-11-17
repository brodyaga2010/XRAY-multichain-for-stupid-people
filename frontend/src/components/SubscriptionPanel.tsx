import { QrCode, RefreshCw, Rss } from "lucide-react";
import { useState } from "react";
import { api } from "../api";
import { CopyButton } from "./CopyButton";
import { QrImage } from "./QrImage";

export function SubscriptionPanel({ setError }: { setError: (error: string) => void }) {
  const [url, setUrl] = useState("");
  const [showQr, setShowQr] = useState(false);
  const [busy, setBusy] = useState(false);

  async function run(action: () => Promise<{ url: string }>) {
    setBusy(true);
    setError("");
    try {
      setUrl((await action()).url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Ошибка запроса");
    } finally {
      setBusy(false);
    }
  }

  function rotate() {
    if (!window.confirm("Старая ссылка-подписка перестанет работать. Перевыпустить?")) return;
    void run(api.rotateSubscription);
  }

  return (
    <section className="tool-panel subscription-panel">
      <div className="panel-heading">
        <h2>Подписка</h2>
      </div>
      <p className="muted">
        Подписку понимают Hiddify, v2rayNG, v2rayN, Streisand, Happ — приложение само подтянет все ваши ключи и
        обновления.
      </p>
      {url ? (
        <>
          <code className="link-uri">{url}</code>
          <div className="link-actions">
            <CopyButton text={url} />
            <button type="button" className="small ghost" onClick={() => setShowQr((current) => !current)}>
              <QrCode size={15} />
              <span>QR-код</span>
            </button>
            <button type="button" className="small ghost danger" disabled={busy} onClick={rotate}>
              <RefreshCw size={15} />
              <span>Перевыпустить</span>
            </button>
          </div>
          {showQr && <QrImage value={url} />}
        </>
      ) : (
        <div className="link-actions">
          <button type="button" className="small ghost" disabled={busy} onClick={() => run(api.subscription)}>
            <Rss size={15} />
            <span>Показать ссылку-подписку</span>
          </button>
          <button type="button" className="small ghost danger" disabled={busy} onClick={rotate}>
            <RefreshCw size={15} />
            <span>Перевыпустить</span>
          </button>
        </div>
      )}
    </section>
  );
}
