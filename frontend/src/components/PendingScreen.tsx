import { CheckCircle2 } from "lucide-react";
import { User } from "../api";

export function PendingScreen({ user, onLogout }: { user: User; onLogout: () => void }) {
  return (
    <main className="pending-page">
      <div className="pending-box animate-in">
        <CheckCircle2 size={40} />
        <h1>Ожидание подтверждения администратором</h1>
        <p>{user.email} зарегистрирован. Доступ откроется после подтверждения.</p>
        <div className="pending-note">Вы сможете создавать ключи доступа, как только аккаунт будет подтверждён.</div>
        <button className="ghost" onClick={onLogout}>
          Выйти
        </button>
      </div>
    </main>
  );
}
