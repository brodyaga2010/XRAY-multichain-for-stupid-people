import { FormEvent, useState } from "react";
import { LogOut } from "lucide-react";
import { api, User } from "../api";

export function Profile({ user, onLogout }: { user: User; onLogout: () => void }) {
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
    <div className="profile-layout">
      <section className="tool-panel animate-in">
        <div className="panel-heading">
          <h2>Аккаунт</h2>
        </div>
        <dl className="user-facts">
          <div className="fact-wide">
            <dt>Эл. почта</dt>
            <dd className="ellipsis" title={user.email}>
              {user.email}
            </dd>
          </div>
          <div>
            <dt>Роль</dt>
            <dd>{user.role === "admin" ? "Администратор" : "Пользователь"}</dd>
          </div>
          <div>
            <dt>Лимит ключей</dt>
            <dd>{user.key_limit}</dd>
          </div>
        </dl>
        <button type="button" className="ghost danger logout-button" onClick={onLogout}>
          <LogOut size={17} /> Выйти
        </button>
      </section>
      <form className="tool-panel profile-form animate-in" onSubmit={submit}>
        <div className="panel-heading">
          <h2>Смена пароля</h2>
        </div>
        <label>
          Текущий пароль
          <input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" required />
        </label>
        <label>
          Новый пароль
          <input type="password" value={next} onChange={(e) => setNext(e.target.value)} placeholder="От 8 символов" minLength={8} autoComplete="new-password" required />
        </label>
        <label>
          Повторите новый пароль
          <input type="password" value={repeat} onChange={(e) => setRepeat(e.target.value)} minLength={8} autoComplete="new-password" required />
        </label>
        {error && <p className="error">{error}</p>}
        {done && <p className="success">{done}</p>}
        <button className="primary" disabled={busy}>
          {busy ? "Подождите..." : "Сменить пароль"}
        </button>
      </form>
    </div>
  );
}
