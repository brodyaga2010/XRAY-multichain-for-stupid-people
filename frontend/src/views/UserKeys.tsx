import { useEffect, useMemo, useState } from "react";
import { Download, Plus } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { SubscriptionPanel } from "../components/SubscriptionPanel";
import { matchesSearch } from "../utils";

const SEARCH_THRESHOLD = 5;

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
  const [loaded, setLoaded] = useState(false);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [importResult, setImportResult] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [newKeyId, setNewKeyId] = useState<number | null>(null);

  const activeCount = useMemo(() => keys.filter((key) => key.status === "active").length, [keys]);
  const limitReached = activeCount >= user.key_limit;
  const showSearch = isAdmin || keys.length > SEARCH_THRESHOLD;
  const filteredKeys = useMemo(
    () => (showSearch ? keys.filter((key) => matchesSearch([key.name, key.owner_email, key.status, key.uuid], query)) : keys),
    [keys, query, showSearch]
  );

  async function load() {
    const result = isAdmin ? await api.adminKeys() : await api.keys();
    setKeys(result);
    setLoaded(true);
  }

  useEffect(() => {
    load().catch((err) => setError(err.message));
  }, [isAdmin]);

  async function createKey(name: string) {
    setError("");
    const created = await api.createKey(name);
    await load().catch((err) => setError(err.message));
    setFormOpen(false);
    setNewKeyId(created.id);
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

  if (isAdmin) {
    return (
      <div className="stack">
        <KeyTable
          keys={filteredKeys}
          showOwner
          onRevoke={revoke}
          headerExtra={
            <button className="ghost" type="button" disabled={busy} onClick={importKeys} title="Импортировать ключи с сервера">
              <Download size={16} />
              <span className="hide-phone">Импортировать ключи с сервера</span>
              <span className="show-phone">Импорт</span>
            </button>
          }
          beforeList={
            <>
              {importResult && <p className="muted">{importResult}</p>}
              <SearchBox value={query} onChange={setQuery} placeholder="Поиск по названию, владельцу, статусу..." />
            </>
          }
        />
      </div>
    );
  }

  const emptyState = (
    <div className="empty-state">
      <h3>У вас пока нет ключей</h3>
      <p className="muted">
        Ключ привязывается к одному устройству. Назовите устройство (например, «Телефон») — и мы создадим ключ для
        подключения.
      </p>
      <CreateKeyForm onCreate={createKey} autoFocus />
    </div>
  );

  return (
    <div className="stack">
      <KeyTable
        keys={filteredKeys}
        title="Мои устройства"
        onRevoke={revoke}
        showCount={false}
        collapseRevoked
        autoOpenKeyId={newKeyId}
        empty={loaded && keys.length === 0 ? emptyState : undefined}
        headerExtra={
          keys.length > 0 && (
            <button
              className="ghost add-device"
              type="button"
              disabled={limitReached}
              aria-expanded={formOpen}
              title={limitReached ? "Достигнут лимит ключей" : undefined}
              onClick={() => setFormOpen((current) => !current)}
            >
              <Plus size={17} />
              <span>Добавить устройство</span>
              <span className="count-pill">
                {activeCount} из {user.key_limit}
              </span>
            </button>
          )
        }
        beforeList={
          <>
            {limitReached && keys.length > 0 && (
              <p className="muted">Достигнут лимит: {user.key_limit}. Отзовите ненужный ключ, чтобы добавить новое устройство.</p>
            )}
            {formOpen && !limitReached && keys.length > 0 && <CreateKeyForm onCreate={createKey} autoFocus />}
            {showSearch && <SearchBox value={query} onChange={setQuery} placeholder="Поиск ключей" />}
          </>
        }
      />
      <SubscriptionPanel setError={setError} />
    </div>
  );
}
