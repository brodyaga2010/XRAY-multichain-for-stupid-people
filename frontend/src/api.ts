export type UserStatus = "pending" | "approved" | "suspended";
export type UserRole = "user" | "admin";
export type KeyType = "tcp_reality" | "xhttp_reality";
export type KeyStatus = "active" | "revoked";

export interface User {
  id: number;
  email: string;
  role: UserRole;
  status: UserStatus;
  key_limit: number;
  created_at: string;
}

export interface VpnKey {
  id: number;
  owner_id: number;
  uuid: string;
  name: string;
  type: KeyType;
  sni: string;
  status: KeyStatus;
  public_key: string;
  short_id: string;
  connection_uri: string;
  client_config_json: Record<string, unknown> | null;
  created_at: string;
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
    const body = await response.json().catch(() => ({ detail: "Request failed" }));
    throw new Error(body.detail ?? "Request failed");
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
  createKey: (payload: { name: string; type: KeyType; sni?: string }) =>
    request<VpnKey>("/keys", { method: "POST", body: JSON.stringify(payload) }),
  revokeKey: (id: number) => request<{ message: string }>(`/keys/${id}`, { method: "DELETE" }),
  adminUsers: () => request<User[]>("/admin/users"),
  adminKeys: () => request<VpnKey[]>("/admin/keys"),
  adminUserKeys: (userId: number) => request<VpnKey[]>(`/admin/users/${userId}/keys`),
  approveUser: (userId: number) => request<User>(`/admin/users/${userId}/approve`, { method: "POST" }),
  suspendUser: (userId: number) => request<User>(`/admin/users/${userId}/suspend`, { method: "POST" }),
  restoreUser: (userId: number) => request<User>(`/admin/users/${userId}/restore`, { method: "POST" }),
  updateLimit: (userId: number, key_limit: number) =>
    request<User>(`/admin/users/${userId}/limits`, {
      method: "PATCH",
      body: JSON.stringify({ key_limit })
    }),
  adminCreateKey: (userId: number, payload: { name: string; type: KeyType; sni?: string }) =>
    request<VpnKey>(`/admin/users/${userId}/keys`, {
      method: "POST",
      body: JSON.stringify(payload)
    }),
  adminRevokeKey: (keyId: number) =>
    request<{ message: string }>(`/admin/keys/${keyId}`, { method: "DELETE" })
};
