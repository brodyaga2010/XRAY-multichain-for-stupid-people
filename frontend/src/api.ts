export type UserStatus = "pending" | "approved" | "suspended";
export type UserRole = "user" | "admin";
export type KeyType = "multi" | "tcp_reality" | "xhttp_reality";
export type KeyStatus = "active" | "suspended" | "revoked";
export type LinkTransport = "tcp" | "grpc" | "xhttp";

export interface User {
  id: number;
  email: string;
  role: UserRole;
  status: UserStatus;
  key_limit: number;
  created_at: string;
}

export interface KeyLink {
  transport: LinkTransport;
  label: string;
  uri: string;
}

export interface VpnKey {
  id: number;
  owner_id: number;
  owner_email: string | null;
  uuid: string;
  name: string;
  type: KeyType;
  sni: string;
  status: KeyStatus;
  public_key: string;
  short_id: string;
  connection_uri: string;
  links: KeyLink[];
  traffic_up: number;
  traffic_down: number;
  last_seen_at: string | null;
  online: boolean;
  created_at: string;
}

export interface AuditEntry {
  id: number;
  actor_id: number | null;
  actor_email: string | null;
  action: string;
  target_type: string | null;
  target_id: string | number | null;
  audit_metadata: Record<string, unknown> | null;
  created_at: string;
}

export type CheckStatus = "ok" | "warn" | "fail" | "unknown";

export interface MonitorCheck {
  id: string;
  name: string;
  status: CheckStatus;
  value: string;
  detail: string;
  since: string | null;
  checked_at: string | null;
}

export interface MonitorPoint {
  at: string;
  ms: number | null;
  status: CheckStatus;
}

export interface MonitorIncident {
  check: string;
  name: string;
  status: CheckStatus;
  detail: string;
  started_at: string;
  ended_at: string | null;
}

/** Состояние VPN для пользователей: только итог и задержка. */
export interface VpnStatus {
  status: CheckStatus;
  latency_ms: number | null;
  checked_at: string | null;
}

export interface MonitorStatus {
  enabled: boolean;
  interval_seconds: number;
  checked_at: string | null;
  checks: MonitorCheck[];
  history: MonitorPoint[];
  incidents: MonitorIncident[];
}

const API_BASE = import.meta.env.VITE_API_BASE ?? "/api";

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(init.headers ?? {})
    }
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({ detail: "Ошибка запроса" }));
    throw new Error(typeof body.detail === "string" ? body.detail : "Ошибка запроса");
  }
  return response.json() as Promise<T>;
}

export const api = {
  me: () => request<{ user: User }>("/auth/me"),
  register: (email: string, password: string) =>
    request<{ user: User }>("/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password })
    }),
  login: (email: string, password: string) =>
    request<{ user: User }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password })
    }),
  logout: () => request<{ message: string }>("/auth/logout", { method: "POST" }),
  keys: () => request<VpnKey[]>("/keys"),
  vpnStatus: () => request<VpnStatus>("/status"),
  createKey: (name: string) =>
    request<VpnKey>("/keys", { method: "POST", body: JSON.stringify({ name }) }),
  revokeKey: (id: number) => request<{ message: string }>(`/keys/${id}`, { method: "DELETE" }),
  adminUsers: () => request<User[]>("/admin/users"),
  adminKeys: () => request<VpnKey[]>("/admin/keys"),
  adminStatus: () => request<MonitorStatus>("/admin/status"),
  adminUserKeys: (userId: number) => request<VpnKey[]>(`/admin/users/${userId}/keys`),
  approveUser: (userId: number) => request<User>(`/admin/users/${userId}/approve`, { method: "POST" }),
  suspendUser: (userId: number) => request<User>(`/admin/users/${userId}/suspend`, { method: "POST" }),
  restoreUser: (userId: number) => request<User>(`/admin/users/${userId}/restore`, { method: "POST" }),
  updateLimit: (userId: number, key_limit: number) =>
    request<User>(`/admin/users/${userId}/limits`, {
      method: "PATCH",
      body: JSON.stringify({ key_limit })
    }),
  adminCreateKey: (userId: number, name: string) =>
    request<VpnKey>(`/admin/users/${userId}/keys`, {
      method: "POST",
      body: JSON.stringify({ name })
    }),
  adminRevokeKey: (keyId: number) =>
    request<{ message: string }>(`/admin/keys/${keyId}`, { method: "DELETE" }),
  adminImportKeys: () => request<{ imported: number }>("/admin/keys/import", { method: "POST" }),
  adminAudit: (limit = 200) => request<AuditEntry[]>(`/admin/audit?limit=${limit}`),
  subscription: () => request<{ url: string }>("/keys/subscription"),
  rotateSubscription: () => request<{ url: string }>("/keys/subscription/rotate", { method: "POST" }),
  changePassword: (current_password: string, new_password: string) =>
    request<{ message: string }>("/auth/password", {
      method: "POST",
      body: JSON.stringify({ current_password, new_password })
    })
};
