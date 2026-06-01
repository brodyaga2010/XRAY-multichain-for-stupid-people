import { Copy, Download, Trash2 } from "lucide-react";
import { VpnKey } from "../api";
import { downloadJson, formatDate } from "../utils";
import { Status } from "./Status";

export function KeyTable({ keys, onRevoke }: { keys: VpnKey[]; onRevoke: (id: number) => void | Promise<void> }) {
  return (
    <div className="table-panel key-table animate-in">
      <div className="panel-heading">
        <h2>Keys</h2>
        <span>{keys.filter((key) => key.status === "active").length} active</span>
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
            {keys.map((key) => (
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
            {keys.length === 0 && (
              <tr>
                <td colSpan={6} className="empty">
                  No keys found
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
