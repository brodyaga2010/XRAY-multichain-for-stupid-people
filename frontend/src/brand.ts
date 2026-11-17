// Название кабинета задаётся при сборке: VITE_SITE_NAME=kemsekov npm run build.
// Намеренно без слов «VPN» и т.п. — страница входа ничем не выдаёт назначение сервиса.
export const SITE_NAME = (import.meta.env.VITE_SITE_NAME ?? "").trim();
export const PAGE_TITLE = SITE_NAME ? `Личный кабинет ${SITE_NAME}` : "Личный кабинет";
