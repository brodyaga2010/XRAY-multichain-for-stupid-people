import { useEffect, useState } from "react";
import { api, User } from "./api";
import { AuthScreen } from "./components/AuthScreen";
import { PendingScreen } from "./components/PendingScreen";
import { Shell } from "./components/Shell";

export function App() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .me()
      .then((session) => setUser(session.user))
      .catch(() => setUser(null))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="boot">Загрузка...</div>;
  if (!user) return <AuthScreen onAuthed={setUser} />;
  if (user.status !== "approved") return <PendingScreen user={user} onLogout={() => logout(setUser)} />;

  return <Shell user={user} onUserChange={setUser} />;
}

async function logout(setUser: (user: User | null) => void) {
  await api.logout().catch(() => undefined);
  setUser(null);
}
