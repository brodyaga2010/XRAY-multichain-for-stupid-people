import { useEffect, useMemo, useState, type MouseEvent } from "react";
import { CalendarPlus, Check, ChevronRight, Tags, X } from "lucide-react";
import { api, BulkAction, Plan, PlanInput, SubscriptionChange, User, UserStatus, VpnKey } from "@/api";
import { useConfirm } from "@/components/ConfirmDialog";
import { FilterChips } from "@/components/FilterChips";
import { KeyTable } from "@/components/KeyTable";
import { PlansManager } from "@/components/PlansManager";
import { SearchBox } from "@/components/SearchBox";
import { SortableHead } from "@/components/SortableHead";
import { StatusBadge } from "@/components/StatusBadge";
import { SubscriptionSummary } from "@/components/SubscriptionForm";
import { UserAvatar } from "@/components/UserAvatar";
import { UserCard } from "@/components/UserCard";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAutoRefresh, useMediaQuery } from "@/hooks";
import { SortColumn, timeOf, useSort } from "@/lib/sort";
import { paidUntilOrder } from "@/lib/subscription";
import { cn } from "@/lib/utils";
import { formatBytes, formatDate, formatSeen, matchesSearch } from "@/utils";

/** expired — подписка закончилась (у подтверждённых и заблокированных). */
type UserFilter = "all" | UserStatus | "expired";

const userFilters: Array<{ id: UserFilter; label: string }> = [
  { id: "all", label: "Все" },
  { id: "pending", label: "Ждут" },
  { id: "approved", label: "Подтверждены" },
  { id: "expired", label: "Истекла" },
  { id: "suspended", label: "Заблокированы" }
];

const isExpired = (user: User) => user.status !== "pending" && !user.subscription_active;
const matchesFilter = (user: User, filter: UserFilter) =>
  filter === "all" || (filter === "expired" ? isExpired(user) : user.status === filter);

/** Пользователь со сводкой по его ключам (отозванные не считаются). */
type UserRow = User & { activeKeys: number; online: number; traffic: number; lastSeen: string | null };

type UserSort = "email" | "status" | "subscription" | "keys" | "traffic" | "activity" | "created";

const statusOrder: Record<UserStatus, number> = { pending: 0, approved: 1, suspended: 2 };

const userColumns: Record<UserSort, SortColumn<UserRow>> = {
  email: { value: (user) => user.email },
  status: { value: (user) => statusOrder[user.status] },
  subscription: { value: (user) => (user.status === "pending" ? null : paidUntilOrder(user)) },
  keys: { value: (user) => user.activeKeys, firstDir: "desc" },
  traffic: { value: (user) => user.traffic, firstDir: "desc" },
  activity: { value: (user) => (user.online > 0 ? Number.MAX_SAFE_INTEGER : timeOf(user.lastSeen)), firstDir: "desc" },
  created: { value: (user) => timeOf(user.created_at), firstDir: "desc" }
};

function toRows(users: User[], keys: VpnKey[]): UserRow[] {
  const byOwner = new Map<number, VpnKey[]>();
  for (const key of keys) {
    if (key.status === "revoked") continue;
    byOwner.set(key.owner_id, [...(byOwner.get(key.owner_id) ?? []), key]);
  }
  return users
    .map((user) => {
      const own = byOwner.get(user.id) ?? [];
      const lastSeen = own.reduce<string | null>(
        (latest, key) => (key.last_seen_at && (!latest || key.last_seen_at > latest) ? key.last_seen_at : latest),
        null
      );
      return {
        ...user,
        activeKeys: own.filter((key) => key.status === "active").length,
        online: own.filter((key) => key.online).length,
        traffic: own.reduce((sum, key) => sum + key.traffic_down + key.traffic_up, 0),
        lastSeen
      };
    })
    .sort((a, b) => b.created_at.localeCompare(a.created_at)); // при равенстве в сортировке — новые выше
}

/** Кому из выбранных подходит действие: подтвердить — ждущих, восстановить — заблокированных,
 *  заблокировать — всех, кроме уже заблокированных и самого администратора;
 *  продлить — не ждущих, у кого подписка не бессрочная. */
function bulkTargets(users: User[], action: BulkAction, selfId: number) {
  switch (action) {
    case "approve":
      return users.filter((user) => user.status === "pending");
    case "restore":
      return users.filter((user) => user.status === "suspended");
    case "suspend":
      return users.filter((user) => user.status !== "suspended" && user.id !== selfId);
    case "extend":
      return users.filter((user) => user.status !== "pending" && user.paid_until !== null);
  }
}

const stop = (event: MouseEvent) => event.stopPropagation();

export function AdminUsers({
  currentUser,
  setError
}: {
  currentUser: User;
  setError: (error: string) => void;
}) {
  const [users, setUsers] = useState<User[]>([]);
  const [allKeys, setAllKeys] = useState<VpnKey[]>([]);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [plansOpen, setPlansOpen] = useState(false);
  const [openId, setOpenId] = useState<number | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<UserFilter>("all");
  const [newKeyId, setNewKeyId] = useState<number | null>(null);
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState(false);
  const [showSuspended, setShowSuspended] = useState(false);
  const isMobile = useMediaQuery("(max-width: 767px)");
  const confirm = useConfirm();

  const rows = useMemo(() => toRows(users, allKeys), [users, allKeys]);
  const searchedRows = useMemo(
    () => rows.filter((user) => matchesSearch([user.email, user.status, user.role, user.key_limit], query)),
    [rows, query]
  );
  const matchingRows = useMemo(
    () => searchedRows.filter((user) => matchesFilter(user, filter)),
    [searchedRows, filter]
  );
  // Счётчики в фильтрах учитывают поиск; «Все» — сколько видно во вкладке «Все».
  // «Истекла» показываем, только когда такие есть (или фильтр уже выбран).
  const filterOptions = userFilters
    .map((item) => ({
      ...item,
      count:
        item.id === "all"
          ? searchedRows.filter((user) => showSuspended || user.status !== "suspended").length
          : searchedRows.filter((user) => matchesFilter(user, item.id)).length
    }))
    .filter((item) => item.id !== "expired" || item.count > 0 || filter === "expired");
  // В «Все» заблокированные спрятаны под ссылкой, чтобы не мешали; во вкладке «Заблокированы» видны всегда.
  const suspendedCount = filter === "all" ? matchingRows.filter((user) => user.status === "suspended").length : 0;
  const visibleRows = useMemo(
    () => (filter === "all" && !showSuspended ? matchingRows.filter((user) => user.status !== "suspended") : matchingRows),
    [matchingRows, filter, showSuspended]
  );
  const { sorted, sort, toggle } = useSort<UserRow, UserSort>(visibleRows, userColumns, { key: "status", dir: "asc" });

  // Выбор действует только на видимых: сменили фильтр или поиск — скрытые снимаются.
  const checkedUsers = sorted.filter((user) => checked.has(user.id));
  const allChecked = sorted.length > 0 && checkedUsers.length === sorted.length;
  const toApprove = bulkTargets(checkedUsers, "approve", currentUser.id);
  const toSuspend = bulkTargets(checkedUsers, "suspend", currentUser.id);
  const toRestore = bulkTargets(checkedUsers, "restore", currentUser.id);
  const toExtend = bulkTargets(checkedUsers, "extend", currentUser.id);
  const onlyPendingToSuspend = toSuspend.every((user) => user.status === "pending");

  const openUser = users.find((user) => user.id === openId) ?? null;
  const openKeys = useMemo(() => allKeys.filter((key) => key.owner_id === openId), [allKeys, openId]);

  useEffect(() => {
    setChecked((current) => {
      const visible = new Set(visibleRows.map((user) => user.id));
      const next = new Set([...current].filter((id) => visible.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [visibleRows]);

  async function load() {
    const [loadedUsers, loadedKeys, loadedPlans] = await Promise.all([api.adminUsers(), api.adminKeys(), api.adminPlans()]);
    setUsers(loadedUsers);
    setAllKeys(loadedKeys);
    setPlans(loadedPlans);
  }

  useEffect(() => {
    load().catch((err) => setError(err.message));
  }, []);

  useAutoRefresh(load);

  function toggleChecked(userId: number, on: boolean) {
    setChecked((current) => {
      const next = new Set(current);
      if (on) next.add(userId);
      else next.delete(userId);
      return next;
    });
  }

  function toggleAll(on: boolean) {
    setChecked(on ? new Set(sorted.map((user) => user.id)) : new Set());
  }

  function open(user: User | null) {
    setOpenId(user?.id ?? null);
    setNewKeyId(null);
  }

  async function mutate(action: () => Promise<unknown>, fallback = "Не удалось обновить пользователя") {
    setError("");
    try {
      await action();
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : fallback);
    }
  }

  /** Блокировка подтверждённого выключает его ключи — спрашиваем; отклонить заявку можно сразу. */
  async function confirmSuspend(targets: User[]) {
    if (targets.every((user) => user.status === "pending")) return true;
    const single = targets.length === 1 ? targets[0] : null;
    return confirm({
      title: single ? `Заблокировать ${single.email}?` : `Заблокировать пользователей: ${targets.length}?`,
      description: single
        ? "Его ключи перестанут работать. Вернуть доступ можно кнопкой «Восстановить»."
        : "Их ключи перестанут работать. Вернуть доступ можно кнопкой «Восстановить».",
      action: "Заблокировать",
      destructive: true
    });
  }

  async function suspend(user: User) {
    if (!(await confirmSuspend([user]))) return;
    await mutate(() => api.suspendUser(user.id));
  }

  async function runBulk(action: BulkAction, targets: User[]) {
    if (targets.length === 0) return;
    if (action === "suspend" && !(await confirmSuspend(targets))) return;
    setError("");
    setBusy(true);
    try {
      const ids = targets.map((user) => user.id);
      const result = await api.bulkUsers(action, ids, action === "extend" ? 1 : undefined);
      setChecked(new Set());
      if (result.failed.length > 0) {
        const emails = result.failed.map((item) => item.email).join(", ");
        setError(`Не получилось для ${result.failed.length} из ${targets.length}: ${emails}`);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось выполнить действие");
    } finally {
      setBusy(false);
    }
    await load().catch((err) => setError(err.message));
  }

  async function savePlan(plan: PlanInput, id?: number) {
    await api.savePlan(plan, id);
    await load();
  }

  async function changeSubscription(userId: number, change: SubscriptionChange) {
    await mutate(() => api.updateSubscription(userId, change), "Не удалось изменить подписку");
  }

  async function createKey(userId: number, name: string) {
    setError("");
    const created = await api.adminCreateKey(userId, name);
    await load();
    setNewKeyId(created.id);
  }

  const quickActions = (user: UserRow) =>
    user.status === "pending" && (
      <div className="flex shrink-0 gap-1" onClick={stop}>
        <Button
          size="icon-sm"
          variant="outline"
          className="text-success hover:bg-success/10 hover:text-success"
          disabled={busy}
          onClick={() => mutate(() => api.approveUser(user.id))}
          aria-label={`Подтвердить ${user.email}`}
          title="Подтвердить"
        >
          <Check />
        </Button>
        <Button
          size="icon-sm"
          variant="outline"
          className="text-destructive hover:bg-destructive/10 hover:text-destructive"
          disabled={busy}
          onClick={() => mutate(() => api.suspendUser(user.id))}
          aria-label={`Отклонить ${user.email}`}
          title="Отклонить"
        >
          <X />
        </Button>
      </div>
    );

  const planName = (user: User) => plans.find((plan) => plan.id === user.plan_id)?.name ?? null;

  const userNote = (user: UserRow) =>
    user.id === currentUser.id ? "это вы" : user.role === "admin" ? "администратор" : null;

  const activity = (user: UserRow) =>
    user.online > 0 ? (
      <span className="inline-flex items-center gap-1.5 text-success">
        <span className="size-2 rounded-full bg-success ring-3 ring-success/20" />
        В сети{user.online > 1 && `: ${user.online}`}
      </span>
    ) : user.lastSeen ? (
      <span className="text-muted-foreground">{formatSeen(user.lastSeen)}</span>
    ) : (
      <span className="text-muted-foreground/70">не подключался</span>
    );

  return (
    <>
      <Card className="gap-4">
        <CardContent className="@container grid gap-4">
          <div className="flex min-w-0 flex-col-reverse gap-3 md:flex-row md:items-center md:justify-between">
            <FilterChips label="Фильтр по статусу" options={filterOptions} value={filter} onChange={setFilter} className="md:shrink-0" />
            <div className="flex min-w-0 gap-2">
              <SearchBox value={query} onChange={setQuery} placeholder="Почта, статус, роль" className="min-w-0 flex-1 md:w-72 md:flex-initial" />
              <Button variant="outline" onClick={() => setPlansOpen(true)}>
                <Tags />
                Тарифы
              </Button>
            </div>
          </div>

          {sorted.length > 0 && (
            <div className={cn("flex min-h-9 flex-wrap items-center gap-x-3 gap-y-2", checkedUsers.length === 0 && "@3xl:hidden")}>
              <Checkbox
                checked={allChecked ? true : checkedUsers.length > 0 ? "indeterminate" : false}
                onCheckedChange={(value) => toggleAll(value === true)}
                aria-label="Выбрать всех"
                className="@3xl:hidden"
              />
              {checkedUsers.length === 0 ? (
                <button
                  type="button"
                  className="text-sm text-muted-foreground hover:text-foreground @3xl:hidden"
                  onClick={() => toggleAll(true)}
                >
                  Выбрать всех
                </button>
              ) : (
                <>
                  <span className="text-sm font-medium">Выбрано: {checkedUsers.length}</span>
                  <div
                    className={cn(
                      "flex flex-wrap gap-2",
                      isMobile &&
                        "fixed inset-x-3 bottom-[calc(4.75rem+env(safe-area-inset-bottom))] z-40 rounded-xl border bg-popover p-2 shadow-lg [&>*]:flex-1"
                    )}
                  >
                    {toApprove.length > 0 && (
                      <Button disabled={busy} onClick={() => runBulk("approve", toApprove)}>
                        <Check />
                        Подтвердить · {toApprove.length}
                      </Button>
                    )}
                    {toRestore.length > 0 && (
                      <Button variant="outline" disabled={busy} onClick={() => runBulk("restore", toRestore)}>
                        Восстановить · {toRestore.length}
                      </Button>
                    )}
                    {toExtend.length > 0 && (
                      <Button variant="outline" disabled={busy} onClick={() => runBulk("extend", toExtend)}>
                        <CalendarPlus />
                        +1 мес · {toExtend.length}
                      </Button>
                    )}
                    {toSuspend.length > 0 && (
                      <Button variant="destructive" disabled={busy} onClick={() => runBulk("suspend", toSuspend)}>
                        <X />
                        {onlyPendingToSuspend ? "Отклонить" : "Заблокировать"} · {toSuspend.length}
                      </Button>
                    )}
                  </div>
                  <Button variant="ghost" className="ml-auto text-muted-foreground" onClick={() => toggleAll(false)}>
                    Снять выбор
                  </Button>
                </>
              )}
            </div>
          )}

          {sorted.length === 0 ? (
            <p className="py-10 text-center text-sm text-muted-foreground">Пользователи не найдены</p>
          ) : (
            <>
              <div className="hidden overflow-hidden rounded-lg border @3xl:block">
                <Table>
                  <TableHeader className="bg-muted/50">
                    <TableRow>
                      <TableHead className="w-10 pl-4">
                        <Checkbox
                          checked={allChecked ? true : checkedUsers.length > 0 ? "indeterminate" : false}
                          onCheckedChange={(value) => toggleAll(value === true)}
                          aria-label="Выбрать всех"
                        />
                      </TableHead>
                      <SortableHead column="email" sort={sort} onSort={toggle}>
                        Пользователь
                      </SortableHead>
                      <SortableHead column="status" sort={sort} onSort={toggle}>
                        Статус
                      </SortableHead>
                      <SortableHead column="subscription" sort={sort} onSort={toggle}>
                        Подписка
                      </SortableHead>
                      <SortableHead column="keys" sort={sort} onSort={toggle}>
                        Ключи
                      </SortableHead>
                      <SortableHead column="traffic" sort={sort} onSort={toggle} className="hidden @5xl:table-cell">
                        Трафик
                      </SortableHead>
                      <SortableHead column="activity" sort={sort} onSort={toggle}>
                        Активность
                      </SortableHead>
                      <SortableHead column="created" sort={sort} onSort={toggle} className="hidden @6xl:table-cell">
                        Регистрация
                      </SortableHead>
                      <TableHead className="w-0" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {sorted.map((user) => (
                      <TableRow
                        key={user.id}
                        onClick={() => open(user)}
                        data-state={checked.has(user.id) ? "selected" : undefined}
                        className={cn("cursor-pointer", openId === user.id && "bg-accent hover:bg-accent")}
                      >
                        <TableCell className="pl-4" onClick={stop}>
                          <Checkbox
                            checked={checked.has(user.id)}
                            onCheckedChange={(value) => toggleChecked(user.id, value === true)}
                            aria-label={`Выбрать ${user.email}`}
                          />
                        </TableCell>
                        <TableCell className="max-w-72">
                          <div className="flex items-center gap-3">
                            <UserAvatar email={user.email} />
                            <div className="grid min-w-40">
                              <span className="truncate font-medium" title={user.email}>
                                {user.email}
                              </span>
                              {userNote(user) && <span className="text-xs text-muted-foreground">{userNote(user)}</span>}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={user.status} kind="user" />
                        </TableCell>
                        <TableCell className="text-sm tabular-nums">
                          {user.status === "pending" ? (
                            <span className="text-muted-foreground">—</span>
                          ) : (
                            <div className="grid">
                              <SubscriptionSummary user={user} short />
                              {planName(user) && <span className="text-xs text-muted-foreground">{planName(user)}</span>}
                            </div>
                          )}
                        </TableCell>
                        <TableCell className="tabular-nums">
                          {user.activeKeys}
                          <span className="text-muted-foreground"> / {user.key_limit}</span>
                        </TableCell>
                        <TableCell className="hidden tabular-nums @5xl:table-cell">{user.traffic > 0 ? formatBytes(user.traffic) : "—"}</TableCell>
                        <TableCell>{activity(user)}</TableCell>
                        <TableCell className="hidden text-muted-foreground tabular-nums @6xl:table-cell">{formatDate(user.created_at)}</TableCell>
                        <TableCell className="pr-3">
                          <div className="flex justify-end">
                            {quickActions(user) || <ChevronRight className="size-4 text-muted-foreground" />}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>

              <ul className={cn("-mx-2 grid gap-0.5 @3xl:hidden", isMobile && checkedUsers.length > 0 && "pb-28")}>
                {sorted.map((user) => (
                  <li
                    key={user.id}
                    className={cn(
                      "flex items-center gap-1 rounded-lg pl-2 transition-colors hover:bg-muted",
                      checked.has(user.id) && "bg-muted/60"
                    )}
                  >
                    <Checkbox
                      checked={checked.has(user.id)}
                      onCheckedChange={(value) => toggleChecked(user.id, value === true)}
                      aria-label={`Выбрать ${user.email}`}
                    />
                    <button
                      type="button"
                      onClick={() => open(user)}
                      className="flex min-w-0 flex-1 items-center gap-3 rounded-lg py-2 pr-2 pl-2 text-left"
                    >
                      <UserAvatar email={user.email} />
                      <div className="grid min-w-0 flex-1 gap-0.5">
                        <span className="truncate text-sm font-medium" title={user.email}>
                          {user.email}
                        </span>
                        <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
                          <StatusBadge status={user.status} kind="user" className="h-4.5 px-1.5 text-[11px]" />
                          {user.status !== "pending" && (
                            <span className="tabular-nums">
                              ключи {user.activeKeys}/{user.key_limit}
                              {user.traffic > 0 && ` · ${formatBytes(user.traffic)}`}
                            </span>
                          )}
                          {user.status !== "pending" && user.paid_until && <SubscriptionSummary user={user} short />}
                          {userNote(user) && <span>{userNote(user)}</span>}
                        </span>
                      </div>
                      {user.status !== "pending" && <ChevronRight className="size-4 shrink-0 text-muted-foreground" />}
                    </button>
                    <div className="pr-2">{quickActions(user)}</div>
                  </li>
                ))}
              </ul>
            </>
          )}

          {suspendedCount > 0 && (
            <Button
              variant="link"
              className="h-auto justify-self-start p-0 text-muted-foreground"
              onClick={() => setShowSuspended((current) => !current)}
            >
              {showSuspended ? "Скрыть заблокированных" : `Показать заблокированных (${suspendedCount})`}
            </Button>
          )}
        </CardContent>
      </Card>

      <Sheet open={openUser !== null} onOpenChange={(value) => !value && open(null)}>
        <SheetContent className="w-full gap-0 bg-background p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-2xl">
          <SheetHeader className="border-b px-4 py-3 pr-12">
            <SheetTitle>Пользователь</SheetTitle>
            <SheetDescription className="sr-only">Доступ пользователя и его ключи</SheetDescription>
          </SheetHeader>
          {openUser && (
            <div className="min-h-0 flex-1 overflow-y-auto">
              <div className="grid gap-4 p-4 pb-[calc(1rem+env(safe-area-inset-bottom))]">
              <UserCard
                user={openUser}
                keys={openKeys}
                isSelf={openUser.id === currentUser.id}
                onApprove={() => mutate(() => api.approveUser(openUser.id))}
                onSuspend={() => suspend(openUser)}
                onRestore={() => mutate(() => api.restoreUser(openUser.id))}
                onSaveLimit={(value) => mutate(() => api.updateLimit(openUser.id, value))}
                onCreateKey={(name) => createKey(openUser.id, name)}
                plans={plans}
                onChangeSubscription={(change) => changeSubscription(openUser.id, change)}
              />
              <KeyTable
                keys={openKeys}
                onRevoke={(id) => mutate(() => api.adminRevokeKey(id), "Не удалось отозвать ключ")}
                autoOpenKeyId={newKeyId}
                cardsOnly
              />
              </div>
            </div>
          )}
        </SheetContent>
      </Sheet>

      <Sheet open={plansOpen} onOpenChange={setPlansOpen}>
        <SheetContent className="w-full gap-0 bg-background p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-lg">
          <SheetHeader className="border-b px-4 py-3 pr-12">
            <SheetTitle>Тарифы</SheetTitle>
            <SheetDescription className="sr-only">Лимиты ключей и сроки подписки</SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-4">
            <PlansManager
              plans={plans}
              onSave={savePlan}
              onDelete={(plan) => mutate(() => api.deletePlan(plan.id), "Не удалось удалить тариф")}
            />
          </div>
        </SheetContent>
      </Sheet>
    </>
  );
}
