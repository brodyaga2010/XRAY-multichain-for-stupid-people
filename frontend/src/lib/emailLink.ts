export type EmailLink = { kind: "verify" | "reset"; token: string };

const pages: Record<string, EmailLink["kind"]> = {
  "/verify-email": "verify",
  "/reset-password": "reset"
};

/** Ссылка из письма: страница подтверждения почты или нового пароля с токеном; иначе null. */
export function emailLinkFromLocation(pathname: string, search: string): EmailLink | null {
  const kind = pages[pathname.replace(/\/+$/, "")];
  if (!kind) return null;
  return { kind, token: new URLSearchParams(search).get("token") ?? "" };
}
