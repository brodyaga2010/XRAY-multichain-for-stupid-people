import { FormEvent, useState } from "react";
import { UserRound } from "lucide-react";
import { api, User } from "@/api";
import { PAGE_TITLE } from "@/brand";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AuthMode } from "@/types";
import { ThemeToggle } from "./ThemeToggle";

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
    <main className="relative grid min-h-dvh place-items-center p-4">
      <div className="absolute top-3 right-3">
        <ThemeToggle />
      </div>
      <Card className="w-full max-w-sm gap-5 py-6">
        <CardHeader className="px-6">
          <span className="mb-2 grid size-10 place-items-center rounded-xl bg-primary text-primary-foreground">
            <UserRound className="size-5" />
          </span>
          <CardTitle className="text-xl font-semibold">{PAGE_TITLE}</CardTitle>
          <CardDescription>Войдите или зарегистрируйтесь — доступ откроется после подтверждения администратором.</CardDescription>
        </CardHeader>
        <CardContent className="px-6">
          <form className="grid gap-4" onSubmit={submit}>
            <Tabs value={mode} onValueChange={(value) => setMode(value as AuthMode)}>
              <TabsList className="h-9 w-full">
                <TabsTrigger value="login">Вход</TabsTrigger>
                <TabsTrigger value="register">Регистрация</TabsTrigger>
              </TabsList>
            </Tabs>
            <div className="grid gap-2">
              <Label htmlFor="auth-email">Эл. почта</Label>
              <Input
                id="auth-email"
                className="h-10"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                type="email"
                autoComplete="email"
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="auth-password">Пароль</Label>
              <Input
                id="auth-password"
                className="h-10"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                type="password"
                autoComplete={mode === "login" ? "current-password" : "new-password"}
                minLength={8}
                required
              />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button size="lg" className="h-10" disabled={pending}>
              {pending ? "Подождите..." : mode === "login" ? "Войти" : "Создать аккаунт"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
