import { FormEvent, useState } from "react";
import { api, User } from "../api";

export function Profile({ user }: { user: User }) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [repeat, setRepeat] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setDone("");
    if (next !== repeat) {
      setError("Новые пароли не совпадают");
      return;
    }
    setBusy(true);
    try {
      const result = await api.changePassword(current, next);
      setDone(result.message || "Пароль изменён");
      setCurrent("");
      setNext("");
      setRepeat("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Ошибка запроса");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="tool-panel profile-form animate-in" onSubmit={submit}>
      <div className="panel-heading">
        <h2>Смена пароля</h2>
        <span>{user.email}</span>
      </div>
      <input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} placeholder="Текущий пароль" autoComplete="current-password" required />
      <input type="password" value={next} onChange={(e) => setNext(e.target.value)} placeholder="Новый пароль (от 8 символов)" minLength={8} autoComplete="new-password" required />
      <input type="password" value={repeat} onChange={(e) => setRepeat(e.target.value)} placeholder="Повторите новый пароль" minLength={8} autoComplete="new-password" required />
      {error && <p className="error">{error}</p>}
      {done && <p className="success">{done}</p>}
      <button className="primary" disabled={busy}>
        {busy ? "Подождите..." : "Сменить пароль"}
      </button>
    </form>
  );
}
