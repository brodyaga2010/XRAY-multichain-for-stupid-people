import { QrCode, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../api";
import { CopyButton } from "./CopyButton";
import { QrImage } from "./QrImage";

export function SubscriptionPanel({ setError }: { setError: (error: string) => void }) {
  const [url, setUrl] = useState("");
  const [showQr, setShowQr] = useState(false);
  const [busy, setBusy] = useState(false);

  async function run(action: () => Promise<{ url: string }>) {
    setBusy(true);
    try {
      setUrl((await action()).url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Ошибка запроса");
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void run(api.subscription);
  }, []);

  function rotate() {
    if (!window.confirm("Старая ссылка-подписка перестанет работать. Перевыпустить?")) return;
    setError("");
    void run(api.rotateSubscription);
  }

  return (
    <section className="tool-panel subscription-panel">
      <div className="panel-heading">
        <h2>Подписка</h2>
      </div>
      <p className="muted">Одна ссылка для Hiddify, v2rayNG, v2rayN, Streisand, Happ — подтянет все ваши ключи.</p>
      <code className="link-line">{url || "Загрузка ссылки..."}</code>
      <div className="link-buttons">
        <CopyButton text={url} label="Скопировать ссылку" className="ghost" />
        <button type="button" className="ghost" disabled={!url} onClick={() => setShowQr((current) => !current)}>
          <QrCode size={16} />
          <span>QR-код</span>
        </button>
        <button type="button" className="ghost" disabled={busy} onClick={rotate}>
          <RefreshCw size={16} />
          <span>Перевыпустить</span>
        </button>
      </div>
      {showQr && url && <QrImage value={url} />}
    </section>
  );
}
