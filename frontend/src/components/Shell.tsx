import {
  BarChart3,
  KeyRound,
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
import { AdminDashboard } from "../views/AdminDashboard";
import { AdminUsers } from "../views/AdminUsers";
import { UserKeys } from "../views/UserKeys";

const userNav = [{ id: "keys" as const, label: "Keys", icon: KeyRound }];
const adminNav = [
  { id: "dashboard" as const, label: "Dashboard", icon: BarChart3 },
  { id: "admin" as const, label: "Users", icon: Users },
  { id: "keys" as const, label: "Keys", icon: KeyRound }
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
            <span>Xray Control</span>
          </div>
          <button className="icon-button mobile-only" onClick={() => setMenuOpen(false)} title="Close menu">
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
        <button className="sidebar-toggle desktop-only" onClick={toggleSidebar} title={sidebarCollapsed ? "Expand menu" : "Collapse menu"}>
          {sidebarCollapsed ? <PanelLeftOpen size={18} /> : <PanelLeftClose size={18} />}
          <span>{sidebarCollapsed ? "Expand" : "Collapse"}</span>
        </button>
      </aside>
      <main className="workspace">
        <header className="topbar">
          <button className="icon-button menu-launcher" onClick={() => setMenuOpen(true)} title="Open menu">
            <Menu size={18} />
          </button>
          <div className="topbar-copy">
            <div className="topbar-head">
              <h1>{titleFor(view, user)}</h1>
              <span className={`status mobile-session ${user.role === "admin" ? "approved" : "active"}`}>
                {user.role === "admin" ? "Admin" : "Approved"}
              </span>
            </div>
          </div>
          <div className="account">
            <div className="account-meta">
              <span>{user.email}</span>
              <span className={`status ${user.role === "admin" ? "approved" : "active"}`}>
                {user.role === "admin" ? "Admin" : "Approved"}
              </span>
            </div>
            <button className="icon-button account-logout" title="Logout" onClick={logout}>
              <LogOut size={18} />
            </button>
          </div>
        </header>
        {error && <div className="banner error">{error}</div>}
        <section className="view-frame">
          {view === "dashboard" && user.role === "admin" && <AdminDashboard setError={setError} />}
          {view === "admin" && user.role === "admin" && <AdminUsers currentUser={user} setError={setError} />}
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
          <span>Exit</span>
        </button>
      </nav>
    </div>
  );
}

function titleFor(view: AppView, user: User) {
  if (view === "dashboard") return "Dashboard";
  if (view === "admin") return "Users";
  return "Keys";
}
