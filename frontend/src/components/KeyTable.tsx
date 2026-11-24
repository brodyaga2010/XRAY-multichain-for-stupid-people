import { ChartColumn, Link2, Trash2 } from "lucide-react";
import { Fragment, ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { KeyStatus, TrafficDay, VpnKey } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { SortColumn, timeOf, useSort } from "@/lib/sort";
import { cn } from "@/lib/utils";
import { formatDate, keyTitle, shortUuid } from "@/utils";
import { useConfirm } from "./ConfirmDialog";
import { FilterChips } from "./FilterChips";
import { OnlineIndicator, Traffic } from "./KeyActivity";
import { SortableHead } from "./SortableHead";
import { KeyModal } from "./KeyModal";
import { StatusBadge } from "./StatusBadge";
import { TrafficChart } from "./TrafficChart";

type KeyFilter = "all" | KeyStatus;

const allFilters: Array<{ id: KeyFilter; label: string }> = [
  { id: "all", label: "Все" },
  { id: "active", label: "Активные" },
  { id: "suspended", label: "Отключённые" },
  { id: "expired", label: "Истекли" },
  { id: "revoked", label: "Отозванные" }
];
const filtersWithoutRevoked = allFilters.filter((item) => item.id !== "revoked");

type KeySort = "name" | "owner" | "activity" | "traffic" | "created";

const keyColumns: Record<KeySort, SortColumn<VpnKey>> = {
  name: { value: (key) => keyTitle(key) },
  owner: { value: (key) => key.owner_email },
  // Сначала те, кто в сети, потом по времени последнего подключения; отключённые и отозванные — в конце.
  activity: {
    value: (key) => (key.status !== "active" ? null : key.online ? Number.MAX_SAFE_INTEGER : timeOf(key.last_seen_at)),
    firstDir: "desc"
  },
  traffic: { value: (key) => key.traffic_down + key.traffic_up, firstDir: "desc" },
  created: { value: (key) => timeOf(key.created_at), firstDir: "desc" }
};

/** Загрузка истории трафика одного ключа за days дней. */
type KeyTrafficLoader = (keyId: number, days: number) => Promise<TrafficDay[]>;

/** Трафик ключа как кнопка: раскрывает график по дням. Без onToggle — просто цифры. */
function TrafficToggle({
  keyItem,
  open,
  onToggle,
  className
}: {
  keyItem: VpnKey;
  open: boolean;
  onToggle?: () => void;
  className?: string;
}) {
  if (!onToggle) return <Traffic keyItem={keyItem} className={className} />;
  return (
    <button
      type="button"
      className={cn(
        "-mx-1.5 -my-1 inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
        open && "bg-muted"
      )}
      aria-expanded={open}
      title={open ? "Скрыть трафик по дням" : "Трафик по дням"}
      onClick={onToggle}
    >
      <Traffic keyItem={keyItem} className={className} />
      <ChartColumn className={cn("size-3.5 shrink-0", open ? "text-foreground" : "text-muted-foreground")} />
    </button>
  );
}

function KeyTrafficChart({ keyId, load }: { keyId: number; load: KeyTrafficLoader }) {
  const loadKey = useCallback((days: number) => load(keyId, days), [load, keyId]);
  return <TrafficChart load={loadKey} />;
}

const connectClass = "bg-accent text-accent-foreground hover:bg-accent/70";
const deleteClass = "text-muted-foreground hover:bg-destructive/10 hover:text-destructive";

export function KeyTable({
  keys,
  showOwner = false,
  onRevoke,
  title = "Ключи",
  titleExtra,
  showHeader = true,
  headerExtra,
  search,
  beforeList,
  empty,
  collapseRevoked = false,
  autoOpenKeyId = null,
  cardsOnly = false,
  loadTraffic
}: {
  keys: VpnKey[];
  showOwner?: boolean;
  onRevoke: (id: number) => void | Promise<void>;
  title?: string;
  /** Подпись рядом с заголовком (например, «6 из 10»). */
  titleExtra?: ReactNode;
  /** Без заголовка — когда он повторяет название страницы; кнопки из headerExtra встают к поиску. */
  showHeader?: boolean;
  /** Кнопки справа в шапке списка (без шапки — справа от поиска). */
  headerExtra?: ReactNode;
  /** Поле поиска: на ПК стоит справа в одной строке с фильтрами. */
  search?: ReactNode;
  /** Содержимое между шапкой и списком (форма, сообщения). */
  beforeList?: ReactNode;
  /** Заменяет «Ключей нет», когда список пуст. */
  empty?: ReactNode;
  /** Отозванные ключи скрыты под ссылкой «Показать отозванные». */
  collapseRevoked?: boolean;
  /** Открыть окно этого ключа (например, только что созданного). */
  autoOpenKeyId?: number | null;
  /** Всегда карточки (сеткой на широком экране), без таблицы — для личного списка устройств. */
  cardsOnly?: boolean;
  /** История трафика ключа: если задана, трафик в строке раскрывает график по дням. */
  loadTraffic?: KeyTrafficLoader;
}) {
  const [filter, setFilter] = useState<KeyFilter>("all");
  const confirm = useConfirm();
  const [showRevoked, setShowRevoked] = useState(false);
  const [openKeyId, setOpenKeyId] = useState<number | null>(null);
  const [trafficKeyId, setTrafficKeyId] = useState<number | null>(null);

  useEffect(() => {
    if (autoOpenKeyId !== null) setOpenKeyId(autoOpenKeyId);
  }, [autoOpenKeyId]);

  const { sorted, sort, toggle } = useSort<VpnKey, KeySort>(keys, keyColumns, { key: "created", dir: "desc" });
  const revokedKeys = useMemo(() => sorted.filter((key) => key.status === "revoked"), [sorted]);
  const baseKeys = collapseRevoked ? sorted.filter((key) => key.status !== "revoked") : sorted;
  const filtered = baseKeys.filter((key) => filter === "all" || key.status === filter);
  const rendered = collapseRevoked && showRevoked && filter === "all" ? [...filtered, ...revokedKeys] : filtered;
  const openKey = keys.find((key) => key.id === openKeyId) ?? null;
  // Пользователю фильтр нужен, только если в списке есть ключи с разными статусами.
  const showChips = collapseRevoked ? new Set(baseKeys.map((key) => key.status)).size > 1 : keys.length > 0;
  // «Истекли» — только если такие ключи есть (или фильтр уже выбран).
  const filterOptions = (collapseRevoked ? filtersWithoutRevoked : allFilters)
    .map((item) => ({
      ...item,
      count: item.id === "all" ? baseKeys.length : baseKeys.filter((key) => key.status === item.id).length
    }))
    .filter((item) => item.id !== "expired" || item.count > 0 || filter === "expired");
  const toolbarExtra = !showHeader && headerExtra;

  async function confirmRevoke(key: VpnKey) {
    const ok = await confirm({
      title: `Отозвать ключ «${keyTitle(key)}»?`,
      description: "Ключ перестанет работать навсегда, вернуть его нельзя — только создать новый.",
      action: "Отозвать",
      destructive: true
    });
    if (ok) void onRevoke(key.id);
  }

  const trafficToggle = (key: VpnKey) =>
    loadTraffic ? () => setTrafficKeyId((current) => (current === key.id ? null : key.id)) : undefined;

  const status = (key: VpnKey) =>
    key.status === "active" ? <OnlineIndicator keyItem={key} /> : <StatusBadge status={key.status} />;

  return (
    <Card className="gap-4">
      {showHeader && (
        <CardHeader>
          <CardTitle className="flex items-baseline gap-2">
            {title}
            {titleExtra}
          </CardTitle>
          {headerExtra && <CardAction>{headerExtra}</CardAction>}
        </CardHeader>
      )}
      <CardContent className="@container grid gap-4">
        {beforeList}
        {(showChips || search || toolbarExtra) && (
          <div className="flex min-w-0 flex-col-reverse gap-3 md:flex-row md:items-center md:justify-between">
            {showChips ? (
              <FilterChips
                label="Фильтр по статусу"
                options={filterOptions}
                value={filter}
                onChange={setFilter}
                className="md:shrink-0"
              />
            ) : (
              <span />
            )}
            {(search || toolbarExtra) && (
              <div className="flex min-w-0 gap-2">
                {search && <div className="min-w-0 flex-1 md:w-72 md:flex-initial">{search}</div>}
                {toolbarExtra}
              </div>
            )}
          </div>
        )}
        {rendered.length === 0 ? (
          empty ?? <p className="py-10 text-center text-sm text-muted-foreground">Ключей нет</p>
        ) : (
          <>
            {!cardsOnly && (
              <div className="hidden overflow-hidden rounded-lg border @4xl:block">
                <Table>
                  <TableHeader className="bg-muted/50">
                    <TableRow>
                      <SortableHead column="name" sort={sort} onSort={toggle} className="pl-4">
                        Ключ
                      </SortableHead>
                      {showOwner && (
                        <SortableHead column="owner" sort={sort} onSort={toggle}>
                          Владелец
                        </SortableHead>
                      )}
                      <SortableHead column="activity" sort={sort} onSort={toggle}>
                        Подключение
                      </SortableHead>
                      <SortableHead column="traffic" sort={sort} onSort={toggle}>
                        Трафик
                      </SortableHead>
                      <SortableHead column="created" sort={sort} onSort={toggle}>
                        Создан
                      </SortableHead>
                      <TableHead className="w-0" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rendered.map((key) => (
                      <Fragment key={key.id}>
                        <TableRow className={cn(key.status === "revoked" && "opacity-60")}>
                          <TableCell className="max-w-64 pl-4">
                            <div className="grid">
                              <span className="truncate font-medium" title={keyTitle(key)}>
                                {keyTitle(key)}
                              </span>
                              <span className="font-mono text-xs text-muted-foreground">{shortUuid(key.uuid)}</span>
                            </div>
                          </TableCell>
                          {showOwner && (
                            <TableCell className="max-w-56">
                              <span className="block truncate" title={key.owner_email ?? undefined}>
                                {key.owner_email ?? "—"}
                              </span>
                            </TableCell>
                          )}
                          <TableCell>{status(key)}</TableCell>
                          <TableCell>
                            <TrafficToggle keyItem={key} open={trafficKeyId === key.id} onToggle={trafficToggle(key)} />
                          </TableCell>
                          <TableCell className="text-muted-foreground tabular-nums">{formatDate(key.created_at)}</TableCell>
                          <TableCell className="pr-3">
                            {key.status !== "revoked" && (
                              <div className="flex justify-end gap-1">
                                {key.status === "active" && (
                                  <Button size="sm" className={connectClass} onClick={() => setOpenKeyId(key.id)}>
                                    <Link2 />
                                    Подключить
                                  </Button>
                                )}
                                <Button
                                  variant="ghost"
                                  size="icon-sm"
                                  className={deleteClass}
                                  title="Отозвать ключ"
                                  aria-label="Отозвать ключ"
                                  onClick={() => confirmRevoke(key)}
                                >
                                  <Trash2 />
                                </Button>
                              </div>
                            )}
                          </TableCell>
                        </TableRow>
                        {loadTraffic && trafficKeyId === key.id && (
                          <TableRow className="hover:bg-transparent">
                            <TableCell colSpan={showOwner ? 6 : 5} className="px-4 pt-1 pb-4 whitespace-normal">
                              <KeyTrafficChart keyId={key.id} load={loadTraffic} />
                            </TableCell>
                          </TableRow>
                        )}
                      </Fragment>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
            <div className={cn("grid gap-3 @lg:grid-cols-2 @4xl:grid-cols-3", !cardsOnly && "@4xl:hidden")}>
              {rendered.map((key) => (
                <article
                  key={key.id}
                  className={cn(
                    "grid content-start gap-2.5 rounded-xl border bg-card p-3.5 transition-colors hover:border-foreground/20",
                    key.status === "revoked" && "opacity-60"
                  )}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="grid min-w-0 gap-0.5">
                      <strong className="line-clamp-2 font-medium" title={keyTitle(key)}>
                        {keyTitle(key)}
                      </strong>
                      {showOwner && key.owner_email && (
                        <span className="truncate text-xs text-muted-foreground" title={key.owner_email}>
                          {key.owner_email}
                        </span>
                      )}
                    </div>
                    <span className="shrink-0 pt-0.5">{status(key)}</span>
                  </div>
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <TrafficToggle
                      keyItem={key}
                      className="text-foreground"
                      open={trafficKeyId === key.id}
                      onToggle={trafficToggle(key)}
                    />
                    <span>с {formatDate(key.created_at)}</span>
                  </div>
                  {key.status !== "revoked" && (
                    <div className="mt-1 flex items-center gap-2">
                      {key.status === "active" ? (
                        <Button size="lg" className={cn("h-10 flex-1 sm:h-9", connectClass)} onClick={() => setOpenKeyId(key.id)}>
                          <Link2 />
                          Подключить
                        </Button>
                      ) : (
                        <span className="flex-1 text-sm text-muted-foreground">
                          {key.status === "expired" ? "Подписка закончилась" : "Отключён вместе с пользователем"}
                        </span>
                      )}
                      <Button
                        variant="ghost"
                        size="icon-lg"
                        className={cn("size-10 sm:size-9", deleteClass)}
                        title="Отозвать ключ"
                        aria-label="Отозвать ключ"
                        onClick={() => confirmRevoke(key)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  )}
                  {loadTraffic && trafficKeyId === key.id && <KeyTrafficChart keyId={key.id} load={loadTraffic} />}
                </article>
              ))}
            </div>
          </>
        )}
        {collapseRevoked && revokedKeys.length > 0 && (
          <Button variant="link" className="h-auto justify-self-start p-0" onClick={() => setShowRevoked((current) => !current)}>
            {showRevoked ? "Скрыть отозванные" : `Показать отозванные (${revokedKeys.length})`}
          </Button>
        )}
      </CardContent>
      {openKey && <KeyModal keyItem={openKey} onClose={() => setOpenKeyId(null)} />}
    </Card>
  );
}
