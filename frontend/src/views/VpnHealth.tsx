import { useEffect, useState } from "react";
import { api, MonitorStatus } from "@/api";
import { HealthPanel } from "@/components/HealthPanel";
import { useAutoRefresh } from "@/hooks";

/** Вкладка «Состояние»: проверки цепочки, задержка за сутки и последние сбои. */
export function VpnHealth({ setError }: { setError: (error: string) => void }) {
  const [status, setStatus] = useState<MonitorStatus | null>(null);

  useEffect(() => {
    api
      .adminStatus()
      .then(setStatus)
      .catch((err) => setError(err.message));
  }, []);

  useAutoRefresh(() => api.adminStatus().then(setStatus));

  if (!status) return <p className="py-10 text-center text-sm text-muted-foreground">Загрузка…</p>;
  return <HealthPanel status={status} />;
}
