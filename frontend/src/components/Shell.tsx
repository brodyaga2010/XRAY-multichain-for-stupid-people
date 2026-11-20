import { Activity, BarChart3, KeyRound, LogOut, PanelLeftClose, PanelLeftOpen, ScrollText, ShieldCheck, UserCog, Users } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { api, User, VpnStatus } from "@/api";
import { SITE_NAME } from "@/brand";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from "@/components/ui/dropdown-menu";
import { useAutoRefresh } from "@/hooks";
import { cn } from "@/lib/utils";
import { pathForView, viewFromPath } from "@/routes";
import { AppView } from "@/types";
import { AdminDashboard } from "@/views/AdminDashboard";
import { AdminUsers } from "@/views/AdminUsers";
import { AuditLog } from "@/views/AuditLog";
import { VpnHealth } from "@/views/VpnHealth";
import { Profile } from "@/views/Profile";
import { UserKeys } from "@/views/UserKeys";
import { ThemeToggle } from "./ThemeToggle";
import { UserAvatar } from "./UserAvatar";
import { VpnStatusPill } from "./VpnStatusPill";

const userNav = [
  { id: "keys" as const, label: "Ключи", icon: KeyRound },
  { id: "profile" as const, label: "Профиль", icon: UserCog }
];
const adminNav = [
  { id: "dashboard" as const, label: "Обзор", icon: BarChart3 },
  { id: "status" as const, label: "Состояние", short: "VPN", icon: Activity },
  { id: "admin" as const, label: "Пользователи", short: "Люди", icon: Users },
  { id: "keys" as const, label: "Ключи", icon: KeyRound },
  { id: "audit" as const, label: "Журнал", icon: ScrollText },
  { id: "profile" as const, label: "Профиль", icon: UserCog }
];

const titles: Record<AppView, string> = {
  dashboard: "Обзор",
  status: "Состояние VPN",
  admin: "Пользователи",
  keys: "Ключи",
  audit: "Журнал",
  profile: "Профиль"
};

function readCollapsed() {
  try {
    return localStorage.getItem("sidebar-collapsed") === "true";
  } catch {
    return false;
  }
}

export function Shell({ user, onUserChange }: { user: User; onUserChange: (user: User | null) => void }) {
  const isAdmin = user.role === "admin";
  const [view, setView] = useState<AppView>(() => viewFromPath(window.location.pathname, isAdmin));
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [error, setError] = useState("");
  const [vpnStatus, setVpnStatus] = useState<VpnStatus | null>(null);
  const nav = useMemo(() => (isAdmin ? adminNav : userNav), [isAdmin]);

  // Статус VPN в шапке: ошибка загрузки просто прячет индикатор.
  const loadStatus = () => api.vpnStatus().then(setVpnStatus, () => setVpnStatus(null));
  useEffect(() => {
    void loadStatus();
  }, []);
  useAutoRefresh(loadStatus);
  // Подписка, лимит и статус меняются администратором и по сроку — подтягиваем, не перерисовывая без изменений.
  useAutoRefresh(async () => {
    const { user: fresh } = await api.me();
    if (JSON.stringify(fresh) !== JSON.stringify(user)) onUserChange(fresh);
  });

  async function logout() {
    await api.logout().catch(() => undefined);
    onUserChange(null);
  }

  // Адрес всегда соответствует разделу; «Назад»/«Вперёд» в браузере переключают разделы.
  useEffect(() => {
    const canonical = pathForView(view, isAdmin);
    if (window.location.pathname !== canonical) {
      window.history.replaceState(null, "", canonical + window.location.search + window.location.hash);
    }
    const onPopState = () => {
      setView(viewFromPath(window.location.pathname, isAdmin));
      setError("");
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, [isAdmin]);

  function selectView(next: AppView) {
    const path = pathForView(next, isAdmin);
    if (window.location.pathname !== path) window.history.pushState(null, "", path);
    setView(next);
    setError("");
    window.scrollTo({ top: 0 });
  }

  function toggleSidebar() {
    setCollapsed((current) => {
      try {
        localStorage.setItem("sidebar-collapsed", String(!current));
      } catch {
        // просто не запомнится
      }
      return !current;
    });
  }

  return (
    <div className="min-h-dvh md:flex">
      <aside
        className={cn(
          // Иконки стоят на месте (по центру полосы 64 px), при сворачивании меняется только ширина,
          // а подписи уходят под край и гаснут — без перестройки и прыжков.
          "sticky top-0 hidden h-dvh shrink-0 flex-col overflow-hidden border-r bg-sidebar text-sidebar-foreground transition-[width] duration-200 ease-out md:flex",
          collapsed ? "w-16" : "w-60"
        )}
      >
        <div className="flex h-14 items-center gap-2.5 px-4">
          <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-primary-foreground">
            <ShieldCheck className="size-4.5" />
          </span>
          <span className={cn("truncate font-semibold whitespace-nowrap transition-opacity duration-200", collapsed && "opacity-0")}>
            {SITE_NAME || "Кабинет"}
          </span>
        </div>
        <nav className="grid gap-1 px-3 py-2">
          {nav.map((item) => (
            <button
              key={item.id}
              type="button"
              title={collapsed ? item.label : undefined}
              onClick={() => selectView(item.id)}
              aria-current={view === item.id ? "page" : undefined}
              className={cn(
                "flex h-9 items-center gap-2.5 overflow-hidden rounded-md px-3 text-sm font-medium whitespace-nowrap transition-colors",
                "text-sidebar-foreground/75 hover:bg-sidebar-accent/60 hover:text-sidebar-foreground",
                view === item.id && "bg-sidebar-accent text-sidebar-accent-foreground hover:bg-sidebar-accent"
              )}
            >
              <item.icon className="size-4 shrink-0" />
              <span className={cn("transition-opacity duration-200", collapsed && "opacity-0")}>{item.label}</span>
            </button>
          ))}
        </nav>
        <div className="mt-auto p-3">
          <Button
            variant="ghost"
            className="w-full justify-start overflow-hidden px-3 text-sidebar-foreground/75"
            onClick={toggleSidebar}
            title={collapsed ? "Развернуть меню" : "Свернуть меню"}
          >
            {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
            <span className={cn("transition-opacity duration-200", collapsed && "opacity-0")}>Свернуть</span>
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 border-b bg-background/85 backdrop-blur">
          <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-2 px-4 md:px-6">
            <h1 className="truncate text-lg font-semibold">{titles[view]}</h1>
            <div className="ml-auto flex items-center gap-1.5">
              <VpnStatusPill status={vpnStatus} onDetails={isAdmin ? () => selectView("status") : undefined} />
              <ThemeToggle />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon-lg" className="rounded-full" aria-label="Аккаунт">
                    <UserAvatar email={user.email} />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-60">
                  <DropdownMenuLabel className="grid gap-0.5">
                    <span className="truncate text-sm font-medium text-foreground">{user.email}</span>
                    <span className="text-xs font-normal text-muted-foreground">
                      {isAdmin ? "Администратор" : "Пользователь"}
                    </span>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onSelect={() => selectView("profile")}>
                    <UserCog />
                    Профиль
                  </DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onSelect={logout}>
                    <LogOut />
                    Выйти
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </header>

        <main className="mx-auto grid w-full max-w-6xl content-start gap-4 p-4 pb-28 md:gap-6 md:p-6 md:pb-10">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {view === "dashboard" && isAdmin && <AdminDashboard setError={setError} onNavigate={selectView} />}
          {view === "admin" && isAdmin && <AdminUsers currentUser={user} setError={setError} />}
          {view === "status" && isAdmin && <VpnHealth setError={setError} />}
          {view === "audit" && isAdmin && <AuditLog setError={setError} />}
          {view === "profile" && <Profile user={user} onLogout={logout} />}
          {view === "keys" && <UserKeys user={user} isAdmin={isAdmin} setError={setError} />}
        </main>
      </div>

      <nav
        aria-label="Основное меню"
        className="fixed inset-x-0 bottom-0 z-30 grid auto-cols-fr grid-flow-col gap-1 border-t bg-background/90 px-2 pt-1.5 pb-[calc(0.375rem+env(safe-area-inset-bottom))] backdrop-blur md:hidden"
      >
        {nav.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => selectView(item.id)}
            aria-current={view === item.id ? "page" : undefined}
            className={cn(
              "flex min-h-12 min-w-0 flex-col items-center justify-center gap-1 rounded-lg text-[11px] font-medium text-muted-foreground transition-colors",
              view === item.id && "bg-accent text-accent-foreground"
            )}
          >
            <item.icon className="size-5" />
            <span className="max-w-full truncate">{"short" in item ? item.short : item.label}</span>
          </button>
        ))}
      </nav>
    </div>
  );
}
