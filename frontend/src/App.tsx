import { useEffect, useState } from "react";
import { api, User } from "@/api";
import { AuthScreen } from "@/components/AuthScreen";
import { EmailLinkScreen } from "@/components/EmailLinkScreen";
import { PendingScreen } from "@/components/PendingScreen";
import { Shell } from "@/components/Shell";
import { emailLinkFromLocation } from "@/lib/emailLink";

export function App() {
  // Ссылка из письма открывается без входа и до загрузки кабинета.
  const emailLink = emailLinkFromLocation(window.location.pathname, window.location.search);
  if (emailLink) return <EmailLinkScreen link={emailLink} />;
  return <Cabinet />;
}

function Cabinet() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .me()
      .then((session) => setUser(session.user))
      .catch(() => setUser(null))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="grid min-h-dvh place-items-center text-sm text-muted-foreground">Загрузка...</div>;
  if (!user) return <AuthScreen onAuthed={setUser} />;
  if (user.status !== "approved") return <PendingScreen user={user} onLogout={() => logout(setUser)} />;

  return <Shell user={user} onUserChange={setUser} />;
}

async function logout(setUser: (user: User | null) => void) {
  await api.logout().catch(() => undefined);
  setUser(null);
}
