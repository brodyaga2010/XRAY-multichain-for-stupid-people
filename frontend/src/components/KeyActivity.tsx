import { VpnKey } from "../api";
import { formatBytes, formatDateTime } from "../utils";

export function OnlineIndicator({ keyItem }: { keyItem: VpnKey }) {
  if (keyItem.online) {
    return (
      <span className="online-indicator on">
        <i /> В сети
      </span>
    );
  }
  return (
    <span className="online-indicator">
      <i /> {keyItem.last_seen_at ? `Был(а) ${formatDateTime(keyItem.last_seen_at)}` : "Не подключался"}
    </span>
  );
}

export function Traffic({ keyItem }: { keyItem: VpnKey }) {
  return (
    <span className="traffic">
      ↑ {formatBytes(keyItem.traffic_up)} / ↓ {formatBytes(keyItem.traffic_down)}
    </span>
  );
}
