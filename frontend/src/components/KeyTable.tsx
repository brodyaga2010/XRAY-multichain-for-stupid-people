import { Copy, Download, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { VpnKey } from "../api";
import { downloadJson, formatDate } from "../utils";
import { Status } from "./Status";

export function KeyTable({ keys, onRevoke }: { keys: VpnKey[]; onRevoke: (id: number) => void | Promise<void> }) {
  const [activityFilter, setActivityFilter] = useState<"all" | "active" | "revoked">("all");

  const activeCount = useMemo(() => keys.filter((key) => key.status === "active").length, [keys]);
  const visibleKeys = useMemo(() => {
    const filtered = keys.filter((key) => {
      if (activityFilter !== "all" && key.status !== activityFilter) return false;
      return true;
    });

    return filtered.sort((a, b) => {
      const left = new Date(a.created_at).getTime();
      const right = new Date(b.created_at).getTime();
      return right - left;
    });
  }, [activityFilter, keys]);

  return (
    <div className="table-panel key-table animate-in">
      <div className="panel-heading">
        <h2>Keys</h2>
        <span>{activeCount} active</span>
      </div>
      <div className="key-filter-bar">
        <div className="key-filter-segmented">
          <button type="button" className={activityFilter === "all" ? "active" : ""} onClick={() => setActivityFilter("all")}>
            All
          </button>
          <button
            type="button"
            className={activityFilter === "active" ? "active" : ""}
            onClick={() => setActivityFilter("active")}
          >
            Active
          </button>
          <button
            type="button"
            className={activityFilter === "revoked" ? "active" : ""}
            onClick={() => setActivityFilter("revoked")}
          >
            Revoked
          </button>
        </div>
      </div>
      <div className="responsive-table">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>SNI</th>
              <th>Status</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {visibleKeys.map((key) => (
              <tr key={key.id}>
                <td data-label="Name">{key.name}</td>
                <td data-label="Type">{key.type === "tcp_reality" ? "TCP REALITY" : "XHTTP REALITY"}</td>
                <td data-label="SNI">{key.sni}</td>
                <td data-label="Status">
                  <Status status={key.status} />
                </td>
                <td data-label="Created">{formatDate(key.created_at)}</td>
                <td className="row-actions">
                  <button className="icon-button" title="Copy VLESS link" onClick={() => navigator.clipboard.writeText(key.connection_uri)}>
                    <Copy size={17} />
                  </button>
                  {key.client_config_json && (
                    <button className="icon-button" title="Download XHTTP JSON" onClick={() => downloadJson(key)}>
                      <Download size={17} />
                    </button>
                  )}
                  {key.status === "active" && (
                    <button className="icon-button danger" title="Revoke" onClick={() => onRevoke(key.id)}>
                      <Trash2 size={17} />
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {visibleKeys.length === 0 && (
              <tr>
                <td colSpan={6} className="empty">
                  No keys match filters
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <div className="key-cards">
        {visibleKeys.map((key) => (
          <article className="key-card" key={key.id}>
            <div className="key-card-head">
              <div className="key-card-title">
                <strong>{key.name}</strong>
                <span className="key-card-subtitle">
                  {key.type === "tcp_reality" ? "TCP REALITY" : "XHTTP REALITY"}
                  {key.sni ? ` • ${key.sni}` : " • No SNI"}
                </span>
              </div>
              <Status status={key.status} />
            </div>
            <div className="key-card-meta">Created {formatDate(key.created_at)}</div>
            <div className="key-card-actions">
              <button className="ghost action-button" onClick={() => navigator.clipboard.writeText(key.connection_uri)}>
                <Copy size={16} />
                <span>Copy</span>
              </button>
              {key.client_config_json && (
                <button className="ghost action-button" onClick={() => downloadJson(key)}>
                  <Download size={16} />
                  <span>JSON</span>
                </button>
              )}
              {key.status === "active" && (
                <button className="ghost action-button danger" onClick={() => onRevoke(key.id)}>
                  <Trash2 size={16} />
                  <span>Revoke</span>
                </button>
              )}
            </div>
          </article>
        ))}
        {visibleKeys.length === 0 && <div className="empty">No keys match filters</div>}
      </div>
    </div>
  );
}
