import { VpnKey } from "../api";
import { formatBytes, formatSeen } from "../utils";

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
      <i /> {keyItem.last_seen_at ? `Был(а) ${formatSeen(keyItem.last_seen_at)}` : "Не подключался"}
    </span>
  );
}

export function Traffic({ keyItem }: { keyItem: VpnKey }) {
  return (
    <span className="traffic" title="Скачано / отправлено">
      <span>↓ {formatBytes(keyItem.traffic_down)}</span>
      <span>↑ {formatBytes(keyItem.traffic_up)}</span>
    </span>
  );
}
