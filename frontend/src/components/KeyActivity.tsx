import { VpnKey } from "@/api";
import { cn } from "@/lib/utils";
import { formatBytes, formatSeen } from "@/utils";

export function OnlineIndicator({ keyItem, className }: { keyItem: VpnKey; className?: string }) {
  const online = keyItem.online;
  const text = online ? "В сети" : keyItem.last_seen_at ? `Был(а) ${formatSeen(keyItem.last_seen_at)}` : "Не подключался";
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-xs font-medium whitespace-nowrap",
        online ? "text-success" : "text-muted-foreground",
        className
      )}
    >
      <span className={cn("size-2 rounded-full", online ? "bg-success ring-3 ring-success/20" : "bg-muted-foreground/40")} />
      {text}
    </span>
  );
}

export function Traffic({ keyItem, className }: { keyItem: VpnKey; className?: string }) {
  return (
    <span className={cn("inline-flex gap-2.5 text-xs whitespace-nowrap tabular-nums", className)} title="Скачано / отправлено">
      <span>↓ {formatBytes(keyItem.traffic_down)}</span>
      <span className="text-muted-foreground">↑ {formatBytes(keyItem.traffic_up)}</span>
    </span>
  );
}
