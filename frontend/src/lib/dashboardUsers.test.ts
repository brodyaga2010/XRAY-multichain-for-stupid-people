import { describe, expect, it } from "vitest";
import type { User, UserStatus } from "@/api";
import { dashboardUsers } from "./dashboardUsers";

const user = (id: number, status: UserStatus, day: number): User => ({
  id,
  email: `u${id}@example.com`,
  role: "user",
  status,
  key_limit: 2,
  plan_id: null,
  paid_until: null,
  subscription_active: true,
  email_verified: true,
  created_at: `2026-11-${String(day).padStart(2, "0")}T10:00:00Z`
});

describe("dashboardUsers", () => {
  it.each([
    {
      name: "заявки сверху, затем подтверждённые, новые выше",
      users: [user(1, "approved", 1), user(2, "pending", 2), user(3, "approved", 5), user(4, "pending", 4)],
      limit: 5,
      want: [4, 2, 3, 1]
    },
    {
      name: "заблокированные не показываются",
      users: [user(1, "suspended", 9), user(2, "approved", 1), user(3, "suspended", 3)],
      limit: 5,
      want: [2]
    },
    {
      name: "не больше лимита, заявки не вытесняются",
      users: [user(1, "approved", 9), user(2, "approved", 8), user(3, "pending", 1), user(4, "pending", 2)],
      limit: 3,
      want: [4, 3, 1]
    },
    {
      name: "только заблокированные — пусто",
      users: [user(1, "suspended", 1)],
      limit: 5,
      want: []
    },
    { name: "нет пользователей", users: [], limit: 5, want: [] }
  ])("$name", ({ users, limit, want }) => {
    expect(dashboardUsers(users, limit).map((item) => item.id)).toEqual(want);
  });

  it("не меняет исходный список", () => {
    const users = [user(1, "approved", 1), user(2, "approved", 2)];
    dashboardUsers(users);
    expect(users.map((item) => item.id)).toEqual([1, 2]);
  });
});
