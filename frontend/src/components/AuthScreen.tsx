import { FormEvent, useState } from "react";
import { UserRound } from "lucide-react";
import { api, User } from "../api";
import { AuthMode } from "../types";
import { PAGE_TITLE, SITE_NAME } from "../brand";

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
      setError(err instanceof Error ? err.message : "Ошибка запроса");
    } finally {
      setPending(false);
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-copy animate-in">
        <div className="brand-mark">
          <UserRound size={28} />
          {SITE_NAME && <span>{SITE_NAME}</span>}
        </div>
        <h1>{PAGE_TITLE}</h1>
        <p>Войдите или зарегистрируйтесь — доступ откроется после подтверждения администратором.</p>
      </section>
      <form className="auth-panel animate-in delay-1" onSubmit={submit}>
        <div className="segmented">
          <button type="button" className={mode === "login" ? "active" : ""} onClick={() => setMode("login")}>
            Вход
          </button>
          <button type="button" className={mode === "register" ? "active" : ""} onClick={() => setMode("register")}>
            Регистрация
          </button>
        </div>
        <label>
          Эл. почта
          <input
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            type="email"
            autoComplete="email"
            required
          />
        </label>
        <label>
          Пароль
          <input
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            type="password"
            autoComplete={mode === "login" ? "current-password" : "new-password"}
            minLength={8}
            required
          />
        </label>
        {error && <p className="error">{error}</p>}
        <button className="primary" disabled={pending}>
          {pending ? "Подождите..." : mode === "login" ? "Войти" : "Создать аккаунт"}
        </button>
      </form>
    </main>
  );
}
