import { useCallback, useEffect, useMemo, useState } from "react";
import { Download, Plus } from "lucide-react";
import { api, User, VpnKey } from "@/api";
import { Button } from "@/components/ui/button";
import { CreateKeyForm } from "@/components/CreateKeyForm";
import { KeyTable } from "@/components/KeyTable";
import { SearchBox } from "@/components/SearchBox";
import { SubscriptionBanner } from "@/components/SubscriptionBanner";
import { SubscriptionPanel } from "@/components/SubscriptionPanel";
import { TrafficChart } from "@/components/TrafficChart";
import { Card, CardContent } from "@/components/ui/card";
import { useAutoRefresh } from "@/hooks";
import { matchesSearch } from "@/utils";

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
  const expired = !user.subscription_active;
  const canAdd = !limitReached && !expired;
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

  useAutoRefresh(load);
  const loadOwnTraffic = useCallback((days: number) => api.ownTraffic(days), []);

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
      <KeyTable
        keys={filteredKeys}
        showOwner
        onRevoke={revoke}
        showHeader={false}
        headerExtra={
          <Button variant="outline" disabled={busy} onClick={importKeys} title="Импортировать ключи с сервера">
            <Download />
            <span className="max-xl:hidden">Импортировать с сервера</span>
            <span className="xl:hidden">Импорт</span>
          </Button>
        }
        beforeList={importResult && <p className="text-sm text-muted-foreground">{importResult}</p>}
        search={<SearchBox value={query} onChange={setQuery} placeholder="Название или владелец" />}
      />
    );
  }

  const emptyState = (
    <div className="grid justify-items-start gap-3 rounded-xl border border-dashed p-5">
      <h3 className="font-medium">У вас пока нет ключей</h3>
      <p className="text-sm text-muted-foreground">
        Ключ привязывается к одному устройству. Назовите устройство (например, «Телефон») — и мы создадим ключ для
        подключения.
      </p>
      {!expired && (
        <div className="w-full max-w-md">
          <CreateKeyForm onCreate={createKey} autoFocus />
        </div>
      )}
    </div>
  );

  return (
    <div className="grid items-start gap-4 md:gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
      <KeyTable
        keys={filteredKeys}
        cardsOnly
        title="Мои устройства"
        titleExtra={
          keys.length > 0 && (
            <span className="text-sm font-normal text-muted-foreground tabular-nums">
              {activeCount} из {user.key_limit}
            </span>
          )
        }
        onRevoke={revoke}
        collapseRevoked
        autoOpenKeyId={newKeyId}
        empty={loaded && keys.length === 0 ? emptyState : undefined}
        headerExtra={
          keys.length > 0 && (
            <Button
              variant={formOpen ? "secondary" : "default"}
              disabled={!canAdd}
              aria-expanded={formOpen}
              title={expired ? "Подписка закончилась" : limitReached ? "Достигнут лимит ключей" : undefined}
              onClick={() => setFormOpen((current) => !current)}
            >
              <Plus />
              Добавить
            </Button>
          )
        }
        beforeList={
          <>
            <SubscriptionBanner user={user} />
            {limitReached && !expired && keys.length > 0 && (
              <p className="text-sm text-muted-foreground">
                Достигнут лимит: {user.key_limit}. Отзовите ненужный ключ, чтобы добавить новое устройство.
              </p>
            )}
            {formOpen && canAdd && keys.length > 0 && <CreateKeyForm onCreate={createKey} autoFocus />}
          </>
        }
        search={showSearch && <SearchBox value={query} onChange={setQuery} placeholder="Поиск устройств" />}
      />
      <div className="grid gap-4 md:gap-6 xl:sticky xl:top-20">
        <SubscriptionPanel setError={setError} />
        {keys.length > 0 && (
          <Card>
            <CardContent>
              <TrafficChart load={loadOwnTraffic} title="Мой трафик" />
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
