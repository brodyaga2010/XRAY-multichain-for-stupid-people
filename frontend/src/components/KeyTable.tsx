import { Link2, Trash2 } from "lucide-react";
import { ReactNode, useEffect, useMemo, useState } from "react";
import { KeyStatus, VpnKey } from "../api";
import { formatDate, keyTitle, shortUuid } from "../utils";
import { FilterChips } from "./FilterChips";
import { OnlineIndicator, Traffic } from "./KeyActivity";
import { KeyModal } from "./KeyModal";
import { Status } from "./Status";

type KeyFilter = "all" | KeyStatus;

const allFilters: Array<{ id: KeyFilter; label: string }> = [
  { id: "all", label: "Все" },
  { id: "active", label: "Активные" },
  { id: "suspended", label: "Отключённые" },
  { id: "revoked", label: "Отозванные" }
];
const filtersWithoutRevoked = allFilters.filter((item) => item.id !== "revoked");

export function KeyTable({
  keys,
  showOwner = false,
  onRevoke,
  title = "Ключи",
  titleExtra,
  showCount = true,
  headerExtra,
  beforeList,
  empty,
  collapseRevoked = false,
  autoOpenKeyId = null,
  cardsOnly = false
}: {
  keys: VpnKey[];
  showOwner?: boolean;
  onRevoke: (id: number) => void | Promise<void>;
  title?: string;
  /** Подпись рядом с заголовком (например, «6 из 10»). */
  titleExtra?: ReactNode;
  showCount?: boolean;
  /** Кнопки справа в шапке списка. */
  headerExtra?: ReactNode;
  /** Содержимое между шапкой и списком (форма, поиск). */
  beforeList?: ReactNode;
  /** Заменяет «Ключей нет», когда список пуст. */
  empty?: ReactNode;
  /** Отозванные ключи скрыты под ссылкой «Показать отозванные». */
  collapseRevoked?: boolean;
  /** Открыть модалку этого ключа (например, только что созданного). */
  autoOpenKeyId?: number | null;
  /** Всегда карточки (сеткой на широком экране), без таблицы — для личного списка устройств. */
  cardsOnly?: boolean;
}) {
  const [filter, setFilter] = useState<KeyFilter>("all");
  const [showRevoked, setShowRevoked] = useState(false);
  const [openKeyId, setOpenKeyId] = useState<number | null>(null);

  useEffect(() => {
    if (autoOpenKeyId !== null) setOpenKeyId(autoOpenKeyId);
  }, [autoOpenKeyId]);

  const sorted = useMemo(
    () => [...keys].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()),
    [keys]
  );
  const activeCount = useMemo(() => keys.filter((key) => key.status === "active").length, [keys]);
  const revokedKeys = useMemo(() => sorted.filter((key) => key.status === "revoked"), [sorted]);
  const baseKeys = collapseRevoked ? sorted.filter((key) => key.status !== "revoked") : sorted;
  const filtered = baseKeys.filter((key) => filter === "all" || key.status === filter);
  const rendered = collapseRevoked && showRevoked && filter === "all" ? [...filtered, ...revokedKeys] : filtered;
  const openKey = keys.find((key) => key.id === openKeyId) ?? null;
  // Пользователю фильтр нужен, только если в списке есть ключи с разными статусами.
  const showChips = collapseRevoked ? new Set(baseKeys.map((key) => key.status)).size > 1 : keys.length > 0;

  function confirmRevoke(key: VpnKey) {
    if (window.confirm(`Отозвать ключ «${keyTitle(key)}»?`)) void onRevoke(key.id);
  }

  return (
    <div className={`table-panel key-table ${cardsOnly ? "cards-only" : ""}`}>
      <div className="panel-heading">
        <h2>
          {title}
          {titleExtra}
        </h2>
        <div className="panel-heading-side">
          {showCount && <span>активных: {activeCount}</span>}
          {headerExtra}
        </div>
      </div>
      {beforeList}
      {showChips && (
        <FilterChips
          label="Фильтр по статусу"
          options={collapseRevoked ? filtersWithoutRevoked : allFilters}
          value={filter}
          onChange={setFilter}
        />
      )}
      {rendered.length === 0 ? (
        empty ?? <div className="empty">Ключей нет</div>
      ) : (
        <>
          <div className="responsive-table">
            <table>
              <thead>
                <tr>
                  <th className="col-name">Ключ</th>
                  {showOwner && <th>Владелец</th>}
                  <th>Подключение</th>
                  <th>Трафик</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {rendered.map((key) => (
                  <tr key={key.id} className={key.status === "revoked" ? "dimmed" : ""}>
                    <td className="col-name">
                      <div className="table-key">
                        <strong className="clamp-2" title={keyTitle(key)}>
                          {keyTitle(key)}
                        </strong>
                        <span className="uuid">
                          {showOwner ? `${shortUuid(key.uuid)} · ` : ""}с {formatDate(key.created_at)}
                        </span>
                      </div>
                    </td>
                    {showOwner && (
                      <td>
                        <span className="ellipsis owner-cell" title={key.owner_email ?? undefined}>
                          {key.owner_email ?? "—"}
                        </span>
                      </td>
                    )}
                    <td>
                      <div className="status-cell">
                        {key.status === "active" ? <OnlineIndicator keyItem={key} /> : <Status status={key.status} />}
                      </div>
                    </td>
                    <td>
                      <Traffic keyItem={key} />
                    </td>
                    <td>
                      {key.status !== "revoked" && (
                        <div className="row-actions">
                          {key.status === "active" && (
                            <button className="connect-button compact" onClick={() => setOpenKeyId(key.id)}>
                              <Link2 size={15} /> Подключить
                            </button>
                          )}
                          <button
                            className="icon-button danger"
                            title="Отозвать"
                            aria-label="Отозвать ключ"
                            onClick={() => confirmRevoke(key)}
                          >
                            <Trash2 size={17} />
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="key-cards">
            {rendered.map((key) => (
              <article className={`key-card ${key.status === "revoked" ? "dimmed" : ""}`} key={key.id}>
                <div className="key-card-head">
                  <div className="key-card-title">
                    <strong className="clamp-2" title={keyTitle(key)}>
                      {keyTitle(key)}
                    </strong>
                    {showOwner && key.owner_email && (
                      <span className="key-card-subtitle ellipsis" title={key.owner_email}>
                        {key.owner_email}
                      </span>
                    )}
                  </div>
                  {key.status === "active" ? <OnlineIndicator keyItem={key} /> : <Status status={key.status} />}
                </div>
                <div className="key-card-line">
                  <Traffic keyItem={key} />
                  <span>с {formatDate(key.created_at)}</span>
                </div>
                {key.status !== "revoked" && (
                  <div className="key-card-actions">
                    {key.status === "active" ? (
                      <button className="connect-button" onClick={() => setOpenKeyId(key.id)}>
                        <Link2 size={17} />
                        <span>Подключить</span>
                      </button>
                    ) : (
                      <span className="muted key-card-note">Отключён вместе с пользователем</span>
                    )}
                    <button
                      className="icon-button danger"
                      title="Отозвать ключ"
                      aria-label="Отозвать ключ"
                      onClick={() => confirmRevoke(key)}
                    >
                      <Trash2 size={18} />
                    </button>
                  </div>
                )}
              </article>
            ))}
          </div>
        </>
      )}
      {collapseRevoked && revokedKeys.length > 0 && (
        <button type="button" className="link-button" onClick={() => setShowRevoked((current) => !current)}>
          {showRevoked ? "Скрыть отозванные" : `Показать отозванные (${revokedKeys.length})`}
        </button>
      )}
      {openKey && <KeyModal keyItem={openKey} onClose={() => setOpenKeyId(null)} />}
    </div>
  );
}
