import { QrCode, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { CopyButton } from "./CopyButton";
import { QrImage } from "./QrImage";

export function SubscriptionPanel({ setError, className }: { setError: (error: string) => void; className?: string }) {
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
    <Card className={cn("gap-4", className)}>
      <CardHeader>
        <CardTitle>Подписка</CardTitle>
        <CardDescription>Одна ссылка для Hiddify, v2rayNG, v2rayN, Streisand, Happ — подтянет все ваши ключи.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        <code className="truncate rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs">{url || "Загрузка ссылки..."}</code>
        <div className="grid grid-cols-2 gap-2">
          <CopyButton text={url} label="Скопировать" size="lg" className="col-span-2" />
          <Button type="button" variant="outline" size="lg" disabled={!url} onClick={() => setShowQr((current) => !current)}>
            <QrCode />
            QR-код
          </Button>
          <Button type="button" variant="outline" size="lg" disabled={busy} onClick={rotate}>
            <RefreshCw />
            Перевыпустить
          </Button>
        </div>
        {showQr && url && <QrImage value={url} />}
      </CardContent>
    </Card>
  );
}
