import { FormEvent, useEffect, useRef, useState } from "react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { EmailLink } from "@/lib/emailLink";
import { AuthLayout } from "./AuthScreen";

const toCabinet = () => window.location.assign("/");

/** Страница из письма: подтверждение почты (сразу) или новый пароль (форма). */
export function EmailLinkScreen({ link }: { link: EmailLink }) {
  return link.kind === "verify" ? <VerifyEmail token={link.token} /> : <ResetPassword token={link.token} />;
}

function Result({ ok, text }: { ok: boolean; text: string }) {
  return <p className={ok ? "rounded-lg bg-success/10 p-3 text-sm text-success" : "rounded-lg bg-destructive/10 p-3 text-sm text-destructive"}>{text}</p>;
}

function VerifyEmail({ token }: { token: string }) {
  const [state, setState] = useState<{ ok: boolean; text: string } | null>(null);
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return; // в StrictMode эффект запускается дважды, а ссылка одноразовая
    started.current = true;
    api
      .verifyEmail(token)
      .then((result) => setState({ ok: true, text: result.message }))
      .catch((err) => setState({ ok: false, text: err instanceof Error ? err.message : "Не удалось подтвердить почту" }));
  }, [token]);

  return (
    <AuthLayout title="Подтверждение почты" description="Проверяем ссылку из письма.">
      <div className="grid gap-4">
        {state ? <Result {...state} /> : <p className="text-sm text-muted-foreground">Подождите...</p>}
        <Button size="lg" className="h-10" onClick={toCabinet}>
          Перейти в кабинет
        </Button>
      </div>
    </AuthLayout>
  );
}

function ResetPassword({ token }: { token: string }) {
  const [password, setPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState("");
  const mismatch = repeat.length > 0 && repeat !== password;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (mismatch) return;
    setPending(true);
    setError("");
    try {
      setDone((await api.resetPassword(token, password)).message);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось сменить пароль");
    } finally {
      setPending(false);
    }
  }

  return (
    <AuthLayout title="Новый пароль" description="Придумайте пароль от 8 символов. На других устройствах нужно будет войти заново.">
      {done ? (
        <div className="grid gap-4">
          <Result ok text={done} />
          <Button size="lg" className="h-10" onClick={toCabinet}>
            Войти
          </Button>
        </div>
      ) : (
        <form className="grid gap-4" onSubmit={submit}>
          <div className="grid gap-2">
            <Label htmlFor="reset-password">Новый пароль</Label>
            <Input
              id="reset-password"
              className="h-10"
              type="password"
              autoComplete="new-password"
              minLength={8}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoFocus
              required
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="reset-repeat">Ещё раз</Label>
            <Input
              id="reset-repeat"
              className="h-10"
              type="password"
              autoComplete="new-password"
              value={repeat}
              onChange={(event) => setRepeat(event.target.value)}
              aria-invalid={mismatch || undefined}
              required
            />
            {mismatch && <p className="text-sm text-destructive">Пароли не совпадают</p>}
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button size="lg" className="h-10" disabled={pending || mismatch || !token}>
            {pending ? "Сохраняем..." : "Сохранить пароль"}
          </Button>
          {!token && <Result ok={false} text="В ссылке нет токена — откройте её из письма целиком." />}
        </form>
      )}
    </AuthLayout>
  );
}
