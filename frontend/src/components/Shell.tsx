import {
  BarChart3,
  KeyRound,
  ScrollText,
  UserCog,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  ShieldCheck,
  Users
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { api, User, VpnStatus } from "../api";
import { useAutoRefresh } from "../hooks";
import { AppView } from "../types";
import { SITE_NAME } from "../brand";
import { pathForView, viewFromPath } from "../routes";
import { AdminDashboard } from "../views/AdminDashboard";
import { AdminUsers } from "../views/AdminUsers";
import { AuditLog } from "../views/AuditLog";
import { Profile } from "../views/Profile";
import { UserKeys } from "../views/UserKeys";
import { VpnStatusPill } from "./VpnStatusPill";

const userNav = [
  { id: "keys" as const, label: "Ключи", icon: KeyRound },
  { id: "profile" as const, label: "Профиль", icon: UserCog }
];
const adminNav = [
  { id: "dashboard" as const, label: "Обзор", icon: BarChart3 },
  { id: "admin" as const, label: "Пользователи", short: "Люди", icon: Users },
  { id: "keys" as const, label: "Ключи", icon: KeyRound },
  { id: "audit" as const, label: "Журнал", icon: ScrollText },
  { id: "profile" as const, label: "Профиль", icon: UserCog }
];

export function Shell({ user, onUserChange }: { user: User; onUserChange: (user: User | null) => void }) {
  const isAdmin = user.role === "admin";
  const [view, setView] = useState<AppView>(() => viewFromPath(window.location.pathname, isAdmin));
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem("sidebar-collapsed") === "true");
  const [error, setError] = useState("");
  const [vpnStatus, setVpnStatus] = useState<VpnStatus | null>(null);
  const nav = useMemo(() => (user.role === "admin" ? adminNav : userNav), [user.role]);

  // Статус VPN в шапке: ошибка загрузки просто прячет индикатор.
  const loadStatus = () => api.vpnStatus().then(setVpnStatus, () => setVpnStatus(null));
  useEffect(() => {
    void loadStatus();
  }, []);
  useAutoRefresh(loadStatus);

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
    setSidebarCollapsed((current) => {
      localStorage.setItem("sidebar-collapsed", String(!current));
      return !current;
    });
  }

  return (
    <div className={`app-shell ${sidebarCollapsed ? "sidebar-collapsed" : ""} ${isAdmin ? "" : "user-mode"}`}>
      <aside className="sidebar">
        <div className="brand-row">
          <div className="brand-mark compact">
            <ShieldCheck size={24} />
            <span>{SITE_NAME || "Кабинет"}</span>
          </div>
        </div>
        <nav>
          {nav.map((item) => (
            <button
              key={item.id}
              className={view === item.id ? "active" : ""}
              onClick={() => selectView(item.id)}
              title={sidebarCollapsed ? item.label : undefined}
            >
              <item.icon size={18} />
              <span>{item.label}</span>
            </button>
          ))}
        </nav>
        <button className="sidebar-toggle" onClick={toggleSidebar} title={sidebarCollapsed ? "Развернуть меню" : "Свернуть меню"}>
          {sidebarCollapsed ? <PanelLeftOpen size={18} /> : <PanelLeftClose size={18} />}
          <span>{sidebarCollapsed ? "Развернуть" : "Свернуть"}</span>
        </button>
      </aside>
      <main className="workspace">
        <header className="topbar">
          <h1>{titleFor(view)}</h1>
          {isAdmin && <span className="status role-badge approved">Админ</span>}
          <VpnStatusPill status={vpnStatus} onDetails={isAdmin ? () => selectView("dashboard") : undefined} />
          <div className="account">
            <div className="account-meta">
              <span className="ellipsis" title={user.email}>
                {user.email}
              </span>
              <span className={`status ${user.role === "admin" ? "approved" : "active"}`}>
                {user.role === "admin" ? "Админ" : "Подтверждён"}
              </span>
            </div>
            <button className="icon-button account-logout" title="Выйти" aria-label="Выйти" onClick={logout}>
              <LogOut size={18} />
            </button>
          </div>
        </header>
        {error && <div className="banner error">{error}</div>}
        <section className="view-frame">
          {view === "dashboard" && user.role === "admin" && <AdminDashboard setError={setError} onNavigate={selectView} />}
          {view === "admin" && user.role === "admin" && <AdminUsers currentUser={user} setError={setError} />}
          {view === "audit" && user.role === "admin" && <AuditLog setError={setError} />}
          {view === "profile" && <Profile user={user} onLogout={logout} />}
          {view === "keys" && <UserKeys user={user} isAdmin={user.role === "admin"} setError={setError} />}
        </section>
      </main>
      <nav className="bottom-nav" aria-label="Основное меню">
        {nav.map((item) => (
          <button key={item.id} className={view === item.id ? "active" : ""} onClick={() => selectView(item.id)}>
            <item.icon size={20} />
            <span>{"short" in item ? item.short : item.label}</span>
          </button>
        ))}
      </nav>
    </div>
  );
}

function titleFor(view: AppView) {
  if (view === "dashboard") return "Обзор";
  if (view === "admin") return "Пользователи";
  if (view === "audit") return "Журнал";
  if (view === "profile") return "Профиль";
  return "Ключи";
}
