import { FormEvent, useState } from "react";
import { LogOut } from "lucide-react";
import { api, User } from "@/api";
import { ThemeToggleInline } from "@/components/ThemeToggleInline";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";

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
    <div className="grid items-start gap-4 md:grid-cols-2 md:gap-6">
      <Card>
        <CardHeader>
          <CardTitle>Аккаунт</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="flex items-center gap-3">
            <UserAvatar email={user.email} size="lg" />
            <div className="grid min-w-0">
              <span className="truncate font-medium" title={user.email}>
                {user.email}
              </span>
              <span className="text-sm text-muted-foreground">
                {user.role === "admin" ? "Администратор" : "Пользователь"} · лимит ключей {user.key_limit}
              </span>
            </div>
          </div>
          <Separator />
          <div className="flex items-center justify-between gap-3">
            <div className="grid">
              <span className="text-sm font-medium">Тема оформления</span>
              <span className="text-xs text-muted-foreground">Запоминается в этом браузере</span>
            </div>
            <ThemeToggleInline />
          </div>
          <Separator />
          <Button variant="destructive" size="lg" className="justify-self-start" onClick={onLogout}>
            <LogOut />
            Выйти
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Смена пароля</CardTitle>
          <CardDescription>Новый пароль — от 8 символов.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="pw-current">Текущий пароль</Label>
              <Input id="pw-current" type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pw-next">Новый пароль</Label>
              <Input id="pw-next" type="password" value={next} onChange={(e) => setNext(e.target.value)} minLength={8} autoComplete="new-password" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pw-repeat">Повторите новый пароль</Label>
              <Input id="pw-repeat" type="password" value={repeat} onChange={(e) => setRepeat(e.target.value)} minLength={8} autoComplete="new-password" required />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            {done && <p className="text-sm text-success">{done}</p>}
            <Button size="lg" className="justify-self-start" disabled={busy}>
              {busy ? "Подождите..." : "Сменить пароль"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
