import { useEffect, useMemo, useState } from "react";
import { KeyRound, Users } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
import { StatCard } from "../components/StatCard";
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
  const activeCount = useMemo(() => keys.filter((key) => key.status === "active").length, [keys]);
  const filteredKeys = useMemo(
    () => keys.filter((key) => matchesSearch([key.name, key.type, key.sni, key.status, key.uuid], query)),
    [keys, query]
  );

  async function load() {
    const loaded = isAdmin ? await api.adminKeys() : await api.keys();
    setKeys(loaded);
  }

  useEffect(() => {
    load().catch((err) => setError(err.message));
  }, [isAdmin]);

  async function createKey(payload: { name: string; type: "tcp_reality" | "xhttp_reality"; sni?: string }) {
    setBusy(true);
    setError("");
    try {
      await api.createKey(payload);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create key");
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
      setError(err instanceof Error ? err.message : "Failed to revoke key");
    }
  }

  return (
    <div className="stack">
      <div className="metrics-strip non-dashboard-metrics">
        <StatCard label="Active keys" value={activeCount} helper="Currently usable" icon={KeyRound} tone="good" />
        <StatCard
          label={isAdmin ? "Total keys" : "Key limit"}
          value={isAdmin ? keys.length : `${activeCount}/${user.key_limit}`}
          helper={isAdmin ? "Across all users" : "Your active quota"}
          icon={Users}
        />
      </div>
      {!isAdmin ? (
        <div className="key-workspace">
          <CreateKeyForm user={user} activeCount={activeCount} onCreate={createKey} />
          <div className="stack">
            <div className="toolbar-panel">
              <SearchBox value={query} onChange={setQuery} placeholder="Search keys by name, SNI, UUID, type..." />
              {busy && <span className="muted">Updating...</span>}
            </div>
            <KeyTable keys={filteredKeys} onRevoke={revoke} />
          </div>
        </div>
      ) : (
        <>
          <div className="toolbar-panel">
            <SearchBox value={query} onChange={setQuery} placeholder="Search keys by name, SNI, UUID, type..." />
            {busy && <span className="muted">Updating...</span>}
          </div>
          <KeyTable keys={filteredKeys} onRevoke={revoke} />
        </>
      )}
    </div>
  );
}
