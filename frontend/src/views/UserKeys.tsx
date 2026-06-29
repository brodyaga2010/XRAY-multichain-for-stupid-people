import { useEffect, useMemo, useState } from "react";
import { EyeOff, Eye, KeyRound, Users } from "lucide-react";
import { api, User, VpnKey } from "../api";
import { CreateKeyForm } from "../components/CreateKeyForm";
import { KeyTable } from "../components/KeyTable";
import { SearchBox } from "../components/SearchBox";
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
  const [showWidgets, setShowWidgets] = useState(true);
  const isMobile = useMediaQuery("(max-width: 820px)");
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
      {showWidgets && (
        <div className="metrics-strip non-dashboard-metrics collapsible-widgets">
          <StatCard label="Active keys" value={activeCount} helper="Currently usable" icon={KeyRound} tone="good" />
          <StatCard
            label={isAdmin ? "Total keys" : "Key limit"}
            value={isAdmin ? keys.length : `${activeCount}/${user.key_limit}`}
            helper={isAdmin ? "Across all users" : "Your active quota"}
            icon={Users}
          />
        </div>
      )}
      {!isAdmin ? (
        <div className="key-workspace">
          {showWidgets && <CreateKeyForm user={user} activeCount={activeCount} onCreate={createKey} />}
          <div className="stack">
            <div className="toolbar-panel sticky-toolbar">
              <SearchBox value={query} onChange={setQuery} placeholder={isMobile ? "Search keys" : "Search keys by name, SNI, UUID, type..."} />
              <div className="toolbar-actions">
                {busy && <span className="muted">Updating...</span>}
                <button className="small ghost widgets-toggle" type="button" onClick={() => setShowWidgets((current) => !current)}>
                  {showWidgets ? <EyeOff size={15} /> : <Eye size={15} />}
                  <span>{showWidgets ? "Hide widgets" : "Show widgets"}</span>
                </button>
              </div>
            </div>
            <KeyTable keys={filteredKeys} onRevoke={revoke} />
          </div>
        </div>
      ) : (
        <>
          <div className="toolbar-panel sticky-toolbar">
            <SearchBox value={query} onChange={setQuery} placeholder={isMobile ? "Search keys" : "Search keys by name, SNI, UUID, type..."} />
            <div className="toolbar-actions">
              {busy && <span className="muted">Updating...</span>}
              <button className="small ghost widgets-toggle" type="button" onClick={() => setShowWidgets((current) => !current)}>
                {showWidgets ? <EyeOff size={15} /> : <Eye size={15} />}
                <span>{showWidgets ? "Hide widgets" : "Show widgets"}</span>
              </button>
            </div>
          </div>
          <KeyTable keys={filteredKeys} onRevoke={revoke} />
        </>
      )}
    </div>
  );
}
