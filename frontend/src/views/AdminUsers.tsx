import { useEffect, useMemo, useState } from "react";
import { Check, ChevronRight, X } from "lucide-react";
import { api, User, UserStatus, VpnKey } from "@/api";
import { useConfirm } from "@/components/ConfirmDialog";
import { FilterChips } from "@/components/FilterChips";
import { KeyTable } from "@/components/KeyTable";
import { SearchBox } from "@/components/SearchBox";
import { StatusBadge } from "@/components/StatusBadge";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";
import { UserCard } from "@/components/UserCard";
import { useAutoRefresh, useMediaQuery } from "@/hooks";
import { matchesSearch } from "@/utils";

type UserFilter = "all" | UserStatus;

const userFilters: Array<{ id: UserFilter; label: string }> = [
  { id: "all", label: "Все" },
  { id: "pending", label: "Ждут" },
  { id: "approved", label: "Подтверждены" },
  { id: "suspended", label: "Заблокированы" }
];

type BulkAction = "approve" | "suspend" | "restore";

/** Кому из выбранных подходит действие: подтвердить — ждущих, восстановить — заблокированных,
 *  заблокировать — всех, кроме уже заблокированных и самого администратора. */
function bulkTargets(users: User[], action: BulkAction, selfId: number) {
  switch (action) {
    case "approve":
      return users.filter((user) => user.status === "pending");
    case "restore":
      return users.filter((user) => user.status === "suspended");
    case "suspend":
      return users.filter((user) => user.status !== "suspended" && user.id !== selfId);
  }
}

const bulkCalls: Record<BulkAction, (userId: number) => Promise<User>> = {
  approve: api.approveUser,
  suspend: api.suspendUser,
  restore: api.restoreUser
};

export function AdminUsers({
  currentUser,
  setError
}: {
  currentUser: User;
  setError: (error: string) => void;
}) {
  const [users, setUsers] = useState<User[]>([]);
  const [selected, setSelected] = useState<User | null>(null);
  const [keys, setKeys] = useState<VpnKey[]>([]);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<UserFilter>("all");
  const [newKeyId, setNewKeyId] = useState<number | null>(null);
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState(false);
  const [showSuspended, setShowSuspended] = useState(false);
  const isMobile = useMediaQuery("(max-width: 900px)");
  const confirm = useConfirm();

  const matchingUsers = useMemo(
    () =>
      users.filter(
        (user) =>
          (filter === "all" || user.status === filter) &&
          matchesSearch([user.email, user.status, user.role, user.key_limit], query)
      ),
    [users, query, filter]
  );
  // В «Все» заблокированные спрятаны под ссылкой, чтобы не мешали; во вкладке «Заблокированы» видны всегда.
  const suspendedCount = filter === "all" ? matchingUsers.filter((user) => user.status === "suspended").length : 0;
  const filteredUsers = useMemo(
    () => (filter === "all" && !showSuspended ? matchingUsers.filter((user) => user.status !== "suspended") : matchingUsers),
    [matchingUsers, filter, showSuspended]
  );

  // Выбор действует только на видимых: сменили фильтр или поиск — скрытые снимаются.
  const checkedUsers = filteredUsers.filter((user) => checked.has(user.id));
  const allChecked = filteredUsers.length > 0 && checkedUsers.length === filteredUsers.length;
  const toApprove = bulkTargets(checkedUsers, "approve", currentUser.id);
  const toSuspend = bulkTargets(checkedUsers, "suspend", currentUser.id);
  const toRestore = bulkTargets(checkedUsers, "restore", currentUser.id);
  const onlyPendingToSuspend = toSuspend.every((user) => user.status === "pending");

  useEffect(() => {
    setChecked((current) => {
      const visible = new Set(filteredUsers.map((user) => user.id));
      const next = new Set([...current].filter((id) => visible.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [filteredUsers]);

  function toggleChecked(userId: number, on: boolean) {
    setChecked((current) => {
      const next = new Set(current);
      if (on) next.add(userId);
      else next.delete(userId);
      return next;
    });
  }

  function toggleAll(on: boolean) {
    setChecked(on ? new Set(filteredUsers.map((user) => user.id)) : new Set());
  }

  async function loadUsers() {
    const loaded = await api.adminUsers();
    setUsers(loaded);
    setSelected((current) => {
      const match = current ? loaded.find((item) => item.id === current.id) ?? null : null;
      if (match) return match;
      if (isMobile) return null;
      return loaded.find((item) => item.status !== "suspended") ?? loaded[0] ?? null;
    });
  }

  async function loadKeys(userId: number) {
    setKeys(await api.adminUserKeys(userId));
  }

  useEffect(() => {
    loadUsers().catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    if (selected) loadKeys(selected.id).catch((err) => setError(err.message));
  }, [selected?.id]);

  useAutoRefresh(() => Promise.all([loadUsers(), selected ? loadKeys(selected.id) : null]));

  function select(user: User | null) {
    setSelected(user);
    setNewKeyId(null);
    setKeys([]);
    if (isMobile) window.scrollTo({ top: 0 });
  }

  async function mutateUser(action: () => Promise<User>) {
    setError("");
    try {
      await action();
      await loadUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось обновить пользователя");
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
    await mutateUser(() => api.suspendUser(user.id));
  }

  async function runBulk(action: BulkAction, targets: User[]) {
    if (targets.length === 0) return;
    if (action === "suspend" && !(await confirmSuspend(targets))) return;
    setError("");
    setBusy(true);
    const failed: string[] = [];
    for (const user of targets) {
      try {
        await bulkCalls[action](user.id);
      } catch (err) {
        failed.push(`${user.email}: ${err instanceof Error ? err.message : "ошибка"}`);
      }
    }
    setBusy(false);
    setChecked(new Set());
    if (failed.length > 0) setError(`Не получилось для ${failed.length} из ${targets.length}. ${failed.join("; ")}`);
    await loadUsers().catch((err) => setError(err.message));
  }

  async function revokeKey(id: number) {
    if (!selected) return;
    setError("");
    try {
      await api.adminRevokeKey(id);
      await loadKeys(selected.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось отозвать ключ");
    }
  }

  async function createKey(userId: number, name: string) {
    setError("");
    const created = await api.adminCreateKey(userId, name);
    await loadKeys(userId);
    setNewKeyId(created.id);
  }

  const showList = !isMobile || !selected;

  return (
    <div className="grid items-start gap-4 min-[901px]:grid-cols-[minmax(300px,340px)_minmax(0,1fr)] md:gap-6">
      {showList && (
        <Card className="min-w-0 gap-3 min-[901px]:sticky min-[901px]:top-20">
          <CardHeader>
            <CardTitle>Пользователи</CardTitle>
            <CardAction className="self-center text-sm text-muted-foreground">показано: {filteredUsers.length}</CardAction>
          </CardHeader>
          <CardContent className="grid gap-3">
            <SearchBox value={query} onChange={setQuery} placeholder="Почта, статус, роль" />
            <FilterChips
              label="Фильтр по статусу"
              options={userFilters}
              value={filter}
              onChange={setFilter}
              compact
            />
            {filteredUsers.length > 0 && (
              <div className="flex min-h-9 items-center gap-3">
                <Checkbox
                  checked={allChecked ? true : checkedUsers.length > 0 ? "indeterminate" : false}
                  onCheckedChange={(value) => toggleAll(value === true)}
                  aria-label="Выбрать всех"
                />
                {checkedUsers.length === 0 ? (
                  <button type="button" className="text-sm text-muted-foreground hover:text-foreground" onClick={() => toggleAll(true)}>
                    Выбрать всех
                  </button>
                ) : (
                  <>
                    <span className="text-sm font-medium">Выбрано: {checkedUsers.length}</span>
                    <Button variant="ghost" size="sm" className="ml-auto text-muted-foreground" onClick={() => toggleAll(false)}>
                      Снять
                    </Button>
                  </>
                )}
              </div>
            )}
            {checkedUsers.length > 0 && (
              <div
                className={cn(
                  "flex flex-wrap gap-2",
                  isMobile &&
                    "fixed inset-x-3 bottom-[calc(4.75rem+env(safe-area-inset-bottom))] z-40 rounded-xl border bg-popover p-2 shadow-lg [&>*]:flex-1"
                )}
              >
                {toApprove.length > 0 && (
                  <Button size="sm" disabled={busy} onClick={() => runBulk("approve", toApprove)}>
                    <Check />
                    Подтвердить · {toApprove.length}
                  </Button>
                )}
                {toRestore.length > 0 && (
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => runBulk("restore", toRestore)}>
                    Восстановить · {toRestore.length}
                  </Button>
                )}
                {toSuspend.length > 0 && (
                  <Button size="sm" variant="destructive" disabled={busy} onClick={() => runBulk("suspend", toSuspend)}>
                    <X />
                    {onlyPendingToSuspend ? "Отклонить" : "Заблокировать"} · {toSuspend.length}
                  </Button>
                )}
              </div>
            )}
            <ul className={cn("-mx-2 grid gap-0.5", isMobile && checkedUsers.length > 0 && "pb-28")}>
              {filteredUsers.map((item) => (
                <li
                  key={item.id}
                  className={cn(
                    "group flex items-center gap-1 rounded-lg pl-2 transition-colors hover:bg-muted",
                    selected?.id === item.id && "bg-accent hover:bg-accent",
                    checked.has(item.id) && selected?.id !== item.id && "bg-muted/60"
                  )}
                >
                  <Checkbox
                    checked={checked.has(item.id)}
                    onCheckedChange={(value) => toggleChecked(item.id, value === true)}
                    aria-label={`Выбрать ${item.email}`}
                  />
                  <button
                    type="button"
                    onClick={() => select(item)}
                    aria-current={selected?.id === item.id ? "true" : undefined}
                    className="flex min-w-0 flex-1 items-center gap-3 rounded-lg py-2 pr-2 pl-2 text-left"
                  >
                    <UserAvatar email={item.email} />
                    <div className="grid min-w-0 flex-1 gap-0.5">
                      <span className="truncate text-sm font-medium" title={item.email}>
                        {item.email}
                      </span>
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <StatusBadge status={item.status} kind="user" className="h-4.5 px-1.5 text-[11px]" />
                        {item.id === currentUser.id ? "это вы" : item.status !== "pending" && `лимит ${item.key_limit}`}
                      </span>
                    </div>
                    {item.status !== "pending" && <ChevronRight className="size-4 shrink-0 text-muted-foreground md:hidden" />}
                  </button>
                  {item.status === "pending" && (
                    <div className="flex shrink-0 gap-1 pr-2">
                      <Button
                        size="icon-sm"
                        variant="outline"
                        className="text-success hover:bg-success/10 hover:text-success"
                        disabled={busy}
                        onClick={() => mutateUser(() => api.approveUser(item.id))}
                        aria-label={`Подтвердить ${item.email}`}
                        title="Подтвердить"
                      >
                        <Check />
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="outline"
                        className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                        disabled={busy}
                        onClick={() => mutateUser(() => api.suspendUser(item.id))}
                        aria-label={`Отклонить ${item.email}`}
                        title="Отклонить"
                      >
                        <X />
                      </Button>
                    </div>
                  )}
                </li>
              ))}
              {filteredUsers.length === 0 && (
                <li className="py-6 text-center text-sm text-muted-foreground">Пользователи не найдены</li>
              )}
            </ul>
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
      )}
      {selected ? (
        <div className="grid min-w-0 gap-4 md:gap-6">
          <UserCard
            user={selected}
            keys={keys}
            isSelf={selected.id === currentUser.id}
            onBack={isMobile ? () => select(null) : undefined}
            onApprove={() => mutateUser(() => api.approveUser(selected.id))}
            onSuspend={() => suspend(selected)}
            onRestore={() => mutateUser(() => api.restoreUser(selected.id))}
            onSaveLimit={(value) => mutateUser(() => api.updateLimit(selected.id, value))}
            onCreateKey={(name) => createKey(selected.id, name)}
          />
          <KeyTable keys={keys} onRevoke={revokeKey} autoOpenKeyId={newKeyId} />
        </div>
      ) : (
        !isMobile && (
          <Card className="grid min-h-48 place-items-center text-sm text-muted-foreground">
            Выберите пользователя, чтобы управлять доступом и ключами
          </Card>
        )
      )}
    </div>
  );
}
