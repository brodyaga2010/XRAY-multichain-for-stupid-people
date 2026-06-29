import { CheckCircle2 } from "lucide-react";
import { User } from "../api";

export function PendingScreen({ user, onLogout }: { user: User; onLogout: () => void }) {
  return (
    <main className="pending-page">
      <div className="pending-box animate-in">
        <CheckCircle2 size={40} />
        <h1>Waiting for administrator approval</h1>
        <p>{user.email} is registered. Access opens after approval.</p>
        <div className="pending-note">You will be able to create and manage VPN keys as soon as the account status changes to approved.</div>
        <button className="ghost" onClick={onLogout}>
          Logout
        </button>
      </div>
    </main>
  );
}
