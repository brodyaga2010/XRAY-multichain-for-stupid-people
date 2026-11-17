import { Link2, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { KeyStatus, VpnKey } from "../api";
import { formatDate } from "../utils";
import { OnlineIndicator, Traffic } from "./KeyActivity";
import { KeyModal } from "./KeyModal";
import { Status } from "./Status";

type KeyFilter = "all" | KeyStatus;

const filters: Array<{ id: KeyFilter; label: string }> = [
  { id: "all", label: "Все" },
  { id: "active", label: "Активные" },
  { id: "suspended", label: "Отключённые" },
  { id: "revoked", label: "Отозванные" }
];

export function KeyTable({
  keys,
  showOwner = false,
  onRevoke
}: {
  keys: VpnKey[];
  showOwner?: boolean;
  onRevoke: (id: number) => void | Promise<void>;
}) {
  const [activityFilter, setActivityFilter] = useState<KeyFilter>("all");
  const [openKeyId, setOpenKeyId] = useState<number | null>(null);

  const activeCount = useMemo(() => keys.filter((key) => key.status === "active").length, [keys]);
  const openKey = keys.find((key) => key.id === openKeyId) ?? null;
  const visibleKeys = useMemo(() => {
    const filtered = keys.filter((key) => activityFilter === "all" || key.status === activityFilter);
    return filtered.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
  }, [activityFilter, keys]);
  const columns = showOwner ? 7 : 6;

  function confirmRevoke(key: VpnKey) {
    if (window.confirm(`Отозвать ключ «${key.name}»?`)) void onRevoke(key.id);
  }

  return (
    <div className="table-panel key-table animate-in">
      <div className="panel-heading">
        <h2>Ключи</h2>
        <span>активных: {activeCount}</span>
      </div>
      <div className="key-filter-bar">
        <div className="key-filter-segmented">
          {filters.map((item) => (
            <button
              key={item.id}
              type="button"
              className={activityFilter === item.id ? "active" : ""}
              onClick={() => setActivityFilter(item.id)}
            >
              {item.label}
            </button>
          ))}
        </div>
      </div>
      <div className="responsive-table">
        <table>
          <thead>
            <tr>
              <th>Ключ</th>
              {showOwner && <th>Владелец</th>}
              <th>Статус</th>
              <th>Активность</th>
              <th>Трафик</th>
              <th>Создан</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {visibleKeys.map((key) => (
              <tr key={key.id}>
                <td data-label="Ключ">
                  <div className="table-key">
                    <strong>{key.name}</strong>
                  </div>
                </td>
                {showOwner && <td data-label="Владелец">{key.owner_email ?? "—"}</td>}
                <td data-label="Статус">
                  <Status status={key.status} />
                </td>
                <td data-label="Активность">
                  <OnlineIndicator keyItem={key} />
                </td>
                <td data-label="Трафик">
                  <Traffic keyItem={key} />
                </td>
                <td data-label="Создан">{formatDate(key.created_at)}</td>
                <td className="row-actions">
                  {key.status !== "revoked" && (
                    <button className="small ghost" title="Подключить" onClick={() => setOpenKeyId(key.id)}>
                      <Link2 size={15} /> Подключить
                    </button>
                  )}
                  {key.status !== "revoked" && (
                    <button className="icon-button danger" title="Отозвать" onClick={() => confirmRevoke(key)}>
                      <Trash2 size={17} />
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {visibleKeys.length === 0 && (
              <tr>
                <td colSpan={columns} className="empty">
                  Ключей нет
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
                {showOwner && key.owner_email && <span className="key-card-subtitle">{key.owner_email}</span>}
              </div>
              <Status status={key.status} />
            </div>
            <div className="key-card-meta">
              <OnlineIndicator keyItem={key} />
              <Traffic keyItem={key} />
              <span>Создан {formatDate(key.created_at)}</span>
            </div>
            {key.status !== "revoked" && (
              <div className="key-card-actions">
                <button className="ghost action-button" onClick={() => setOpenKeyId(key.id)}>
                  <Link2 size={16} />
                  <span>Подключить</span>
                </button>
                <button className="ghost action-button danger icon-only-mobile" title="Отозвать ключ" onClick={() => confirmRevoke(key)}>
                  <Trash2 size={16} />
                  <span>Отозвать</span>
                </button>
              </div>
            )}
          </article>
        ))}
        {visibleKeys.length === 0 && <div className="empty">Ключей нет</div>}
      </div>
      {openKey && <KeyModal keyItem={openKey} onClose={() => setOpenKeyId(null)} />}
    </div>
  );
}
