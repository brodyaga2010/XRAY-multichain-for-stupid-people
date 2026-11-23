import type { User } from "@/api";

/**
 * Пользователи для обзора: сначала заявки (их можно подтвердить прямо там), затем подтверждённые,
 * внутри — новые выше. Заблокированных на обзоре не показываем — они есть на вкладке «Пользователи».
 */
export function dashboardUsers(users: User[], limit = 5) {
  const byNewest = [...users].sort((left, right) => right.created_at.localeCompare(left.created_at));
  return [
    ...byNewest.filter((user) => user.status === "pending"),
    ...byNewest.filter((user) => user.status === "approved")
  ].slice(0, limit);
}

/** Активные пользователи — подтверждённые; заявки и заблокированные не считаются. */
export function activeUserCount(users: User[]) {
  return users.filter((user) => user.status === "approved").length;
}
