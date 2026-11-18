import { AppView } from "./types";

// Раздел кабинета хранится в адресе: перезагрузка и кнопка «Назад» оставляют пользователя там же.
const viewPaths: Record<AppView, string> = {
  dashboard: "/",
  admin: "/users",
  keys: "/keys",
  audit: "/audit",
  profile: "/profile"
};

const adminViews: AppView[] = ["dashboard", "admin", "keys", "audit", "profile"];
const userViews: AppView[] = ["keys", "profile"];

export function defaultView(isAdmin: boolean): AppView {
  return isAdmin ? "dashboard" : "keys";
}

/** Раздел по адресу; неизвестный адрес или раздел не для этой роли — раздел по умолчанию. */
export function viewFromPath(pathname: string, isAdmin: boolean): AppView {
  const path = pathname.replace(/\/+$/, "") || "/";
  const allowed = isAdmin ? adminViews : userViews;
  return allowed.find((view) => viewPaths[view] === path) ?? defaultView(isAdmin);
}

export function pathForView(view: AppView, isAdmin: boolean): string {
  // У обычного пользователя «/» и так ведёт к ключам — адрес оставляем коротким.
  return !isAdmin && view === defaultView(false) ? "/" : viewPaths[view];
}
