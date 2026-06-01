import { FormEvent, useState } from "react";
import { ShieldCheck } from "lucide-react";
import { api, User } from "../api";
import { AuthMode } from "../types";

export function AuthScreen({ onAuthed }: { onAuthed: (user: User) => void }) {
  const [mode, setMode] = useState<AuthMode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const session = mode === "login" ? await api.login(email, password) : await api.register(email, password);
      onAuthed(session.user);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Request failed");
    } finally {
      setPending(false);
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-copy animate-in">
        <div className="brand-mark">
          <ShieldCheck size={28} />
          <span>Xray Control</span>
        </div>
        <h1>VPN access that stays organized from phone to desktop.</h1>
        <p>Request approval, manage your keys, and keep every access decision visible.</p>
      </section>
      <form className="auth-panel animate-in delay-1" onSubmit={submit}>
        <div className="segmented">
          <button type="button" className={mode === "login" ? "active" : ""} onClick={() => setMode("login")}>
            Login
          </button>
          <button type="button" className={mode === "register" ? "active" : ""} onClick={() => setMode("register")}>
            Register
          </button>
        </div>
        <label>
          Email
          <input value={email} onChange={(event) => setEmail(event.target.value)} type="email" required />
        </label>
        <label>
          Password
          <input
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            type="password"
            minLength={8}
            required
          />
        </label>
        {error && <p className="error">{error}</p>}
        <button className="primary" disabled={pending}>
          {pending ? "Please wait..." : mode === "login" ? "Login" : "Create account"}
        </button>
      </form>
    </main>
  );
}
