import { useEffect, useMemo, useState } from "react";
import { ChevronRight } from "lucide-react";
import { api, User, UserStatus, VpnKey } from "@/api";
import { FilterChips } from "@/components/FilterChips";
import { KeyTable } from "@/components/KeyTable";
import { SearchBox } from "@/components/SearchBox";
import { StatusBadge } from "@/components/StatusBadge";
import { UserAvatar } from "@/components/UserAvatar";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
  const isMobile = useMediaQuery("(max-width: 900px)");

  const filteredUsers = useMemo(
    () =>
      users.filter(
        (user) =>
          (filter === "all" || user.status === filter) &&
          matchesSearch([user.email, user.status, user.role, user.key_limit], query)
      ),
    [users, query, filter]
  );

  async function loadUsers() {
    const loaded = await api.adminUsers();
    setUsers(loaded);
    setSelected((current) => {
      const match = current ? loaded.find((item) => item.id === current.id) ?? null : null;
      if (match) return match;
      if (isMobile) return null;
      return loaded[0] ?? null;
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

  async function suspend(userId: number) {
    if (!window.confirm("Ключи пользователя перестанут работать. Заблокировать?")) return;
    await mutateUser(() => api.suspendUser(userId));
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
            <CardDescription>показано: {filteredUsers.length}</CardDescription>
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
            <ul className="-mx-2 grid gap-0.5">
              {filteredUsers.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    onClick={() => select(item)}
                    aria-current={selected?.id === item.id ? "true" : undefined}
                    className={cn(
                      "flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left transition-colors hover:bg-muted",
                      selected?.id === item.id && "bg-accent hover:bg-accent"
                    )}
                  >
                    <UserAvatar email={item.email} />
                    <div className="grid min-w-0 flex-1 gap-0.5">
                      <span className="truncate text-sm font-medium" title={item.email}>
                        {item.email}
                      </span>
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <StatusBadge status={item.status} kind="user" className="h-4.5 px-1.5 text-[11px]" />
                        {item.id === currentUser.id ? "это вы" : `лимит ${item.key_limit}`}
                      </span>
                    </div>
                    <ChevronRight className="size-4 shrink-0 text-muted-foreground md:hidden" />
                  </button>
                </li>
              ))}
              {filteredUsers.length === 0 && (
                <li className="py-6 text-center text-sm text-muted-foreground">Пользователи не найдены</li>
              )}
            </ul>
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
            onSuspend={() => suspend(selected.id)}
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
