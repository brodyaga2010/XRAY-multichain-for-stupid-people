import {
  BarChart3,
  KeyRound,
  ScrollText,
  UserCog,
  LogOut,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  ShieldCheck,
  Users,
  X
} from "lucide-react";
import { useMemo, useState } from "react";
import { api, User } from "../api";
import { AppView } from "../types";
import { SITE_NAME } from "../brand";
import { AdminDashboard } from "../views/AdminDashboard";
import { AdminUsers } from "../views/AdminUsers";
import { AuditLog } from "../views/AuditLog";
import { Profile } from "../views/Profile";
import { UserKeys } from "../views/UserKeys";

const userNav = [
  { id: "keys" as const, label: "Ключи", icon: KeyRound },
  { id: "profile" as const, label: "Профиль", icon: UserCog }
];
const adminNav = [
  { id: "dashboard" as const, label: "Обзор", icon: BarChart3 },
  { id: "admin" as const, label: "Пользователи", icon: Users },
  { id: "keys" as const, label: "Ключи", icon: KeyRound },
  { id: "audit" as const, label: "Журнал", icon: ScrollText },
  { id: "profile" as const, label: "Профиль", icon: UserCog }
];

export function Shell({ user, onUserChange }: { user: User; onUserChange: (user: User | null) => void }) {
  const [view, setView] = useState<AppView>(user.role === "admin" ? "dashboard" : "keys");
  const [menuOpen, setMenuOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem("sidebar-collapsed") === "true");
  const [error, setError] = useState("");
  const nav = useMemo(() => (user.role === "admin" ? adminNav : userNav), [user.role]);

  async function logout() {
    await api.logout().catch(() => undefined);
    onUserChange(null);
  }

  function selectView(next: AppView) {
    setView(next);
    setMenuOpen(false);
    setError("");
  }

  function toggleSidebar() {
    setSidebarCollapsed((current) => {
      localStorage.setItem("sidebar-collapsed", String(!current));
      return !current;
    });
  }

  return (
    <div className={`app-shell ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}>
      <aside className={`sidebar ${menuOpen ? "open" : ""}`}>
        <div className="brand-row">
          <div className="brand-mark compact">
            <ShieldCheck size={24} />
            <span>{SITE_NAME || "Кабинет"}</span>
          </div>
          <button className="icon-button mobile-only" onClick={() => setMenuOpen(false)} title="Закрыть меню">
            <X size={18} />
          </button>
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
        <button className="sidebar-toggle desktop-only" onClick={toggleSidebar} title={sidebarCollapsed ? "Развернуть меню" : "Свернуть меню"}>
          {sidebarCollapsed ? <PanelLeftOpen size={18} /> : <PanelLeftClose size={18} />}
          <span>{sidebarCollapsed ? "Развернуть" : "Свернуть"}</span>
        </button>
      </aside>
      <main className="workspace">
        <header className="topbar">
          <button className="icon-button menu-launcher" onClick={() => setMenuOpen(true)} title="Открыть меню">
            <Menu size={18} />
          </button>
          <div className="topbar-copy">
            <div className="topbar-head">
              <h1>{titleFor(view)}</h1>
              <span className={`status mobile-session ${user.role === "admin" ? "approved" : "active"}`}>
                {user.role === "admin" ? "Админ" : "Подтверждён"}
              </span>
            </div>
          </div>
          <div className="account">
            <div className="account-meta">
              <span>{user.email}</span>
              <span className={`status ${user.role === "admin" ? "approved" : "active"}`}>
                {user.role === "admin" ? "Админ" : "Подтверждён"}
              </span>
            </div>
            <button className="icon-button account-logout" title="Выйти" onClick={logout}>
              <LogOut size={18} />
            </button>
          </div>
        </header>
        {error && <div className="banner error">{error}</div>}
        <section className="view-frame">
          {view === "dashboard" && user.role === "admin" && <AdminDashboard setError={setError} />}
          {view === "admin" && user.role === "admin" && <AdminUsers currentUser={user} setError={setError} />}
          {view === "audit" && user.role === "admin" && <AuditLog setError={setError} />}
          {view === "profile" && <Profile user={user} />}
          {view === "keys" && <UserKeys user={user} isAdmin={user.role === "admin"} setError={setError} />}
        </section>
      </main>
      <nav className="bottom-nav">
        {nav.map((item) => (
          <button key={item.id} className={view === item.id ? "active" : ""} onClick={() => selectView(item.id)}>
            <item.icon size={19} />
            <span>{item.label}</span>
          </button>
        ))}
        <button onClick={logout}>
          <LogOut size={19} />
          <span>Выйти</span>
        </button>
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
