import { useEffect, useMemo, useState } from "react";
import { Download, EyeOff, Eye, KeyRound, Users } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { SubscriptionPanel } from "../components/SubscriptionPanel";
import { StatCard } from "../components/StatCard";
import { useMediaQuery } from "../hooks";
import { matchesSearch } from "../utils";

export function UserKeys({
  user,
  isAdmin,
  setError
}: {
  user: User;
  isAdmin: boolean;
  setError: (error: string) => void;
}) {
  const [keys, setKeys] = useState<VpnKey[]>([]);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [importResult, setImportResult] = useState("");
  const [showWidgets, setShowWidgets] = useState(true);
  const isMobile = useMediaQuery("(max-width: 820px)");
  const activeCount = useMemo(() => keys.filter((key) => key.status === "active").length, [keys]);
  const filteredKeys = useMemo(
    () => keys.filter((key) => matchesSearch([key.name, key.owner_email, key.status, key.uuid], query)),
    [keys, query]
  );

  async function load() {
    const loaded = isAdmin ? await api.adminKeys() : await api.keys();
    setKeys(loaded);
  }

  useEffect(() => {
    load().catch((err) => setError(err.message));
  }, [isAdmin]);

  async function createKey(name: string) {
    setBusy(true);
    setError("");
    try {
      await api.createKey(name);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось создать ключ");
    } finally {
      setBusy(false);
    }
  }

  async function revoke(id: number) {
    setError("");
    try {
      if (isAdmin) await api.adminRevokeKey(id);
      else await api.revokeKey(id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось отозвать ключ");
    }
  }

  async function importKeys() {
    setBusy(true);
    setError("");
    setImportResult("");
    try {
      const result = await api.adminImportKeys();
      setImportResult(`Импортировано: ${result.imported}`);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось импортировать ключи");
    } finally {
      setBusy(false);
    }
  }

  const searchPlaceholder = isMobile ? "Поиск ключей" : "Поиск по названию, владельцу, статусу...";

  return (
    <div className="stack">
      {showWidgets && (
        <div className="metrics-strip non-dashboard-metrics collapsible-widgets">
          <StatCard label="Активные ключи" value={activeCount} helper="Работают сейчас" icon={KeyRound} tone="good" />
          <StatCard
            label={isAdmin ? "Всего ключей" : "Лимит ключей"}
            value={isAdmin ? keys.length : `${activeCount}/${user.key_limit}`}
            helper={isAdmin ? "У всех пользователей" : "Ваша квота"}
            icon={Users}
          />
        </div>
      )}
      {!isAdmin ? (
        <div className="key-workspace">
          {showWidgets && (
            <div className="stack">
              <CreateKeyForm user={user} activeCount={activeCount} onCreate={createKey} />
              <SubscriptionPanel setError={setError} />
            </div>
          )}
          <div className="stack">
            <div className="toolbar-panel sticky-toolbar">
              <SearchBox value={query} onChange={setQuery} placeholder={searchPlaceholder} />
              <div className="toolbar-actions">
                {busy && <span className="muted">Обновление...</span>}
                <button className="small ghost widgets-toggle" type="button" onClick={() => setShowWidgets((current) => !current)}>
                  {showWidgets ? <EyeOff size={15} /> : <Eye size={15} />}
                  <span>{showWidgets ? "Скрыть виджеты" : "Показать виджеты"}</span>
                </button>
              </div>
            </div>
            <KeyTable keys={filteredKeys} onRevoke={revoke} />
          </div>
        </div>
      ) : (
        <>
          <div className="toolbar-panel sticky-toolbar">
            <SearchBox value={query} onChange={setQuery} placeholder={searchPlaceholder} />
            <div className="toolbar-actions">
              {importResult && <span className="muted">{importResult}</span>}
              {busy && <span className="muted">Обновление...</span>}
              <button className="small ghost" type="button" disabled={busy} onClick={importKeys}>
                <Download size={15} />
                <span>Импортировать ключи с сервера</span>
              </button>
              <button className="small ghost widgets-toggle" type="button" onClick={() => setShowWidgets((current) => !current)}>
                {showWidgets ? <EyeOff size={15} /> : <Eye size={15} />}
                <span>{showWidgets ? "Скрыть виджеты" : "Показать виджеты"}</span>
              </button>
            </div>
          </div>
          <KeyTable keys={filteredKeys} showOwner onRevoke={revoke} />
        </>
      )}
    </div>
  );
}
