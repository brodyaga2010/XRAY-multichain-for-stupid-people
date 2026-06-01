import { CheckCircle2 } from "lucide-react";
import { User } from "../api";

export function PendingScreen({ user, onLogout }: { user: User; onLogout: () => void }) {
  return (
    <main className="pending-page">
      <div className="pending-box animate-in">
        <CheckCircle2 size={40} />
        <h1>Waiting for administrator approval</h1>
        <p>{user.email} is registered. Access opens after approval.</p>
        <button className="ghost" onClick={onLogout}>
          Logout
        </button>
      </div>
    </main>
  );
}
