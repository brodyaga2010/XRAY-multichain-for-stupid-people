import { FormEvent, useState } from "react";
import { MailCheck, MailWarning } from "lucide-react";
import { api, User } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";

/** Почта в профиле: подтверждена ли, письмо ещё раз, смена адреса по ссылке. */
export function EmailSettings({ user }: { user: User }) {
  const [resend, setResend] = useState<{ ok: boolean; text: string } | null>(null);
  const [resending, setResending] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null);

  async function sendAgain() {
    setResending(true);
    try {
      setResend({ ok: true, text: (await api.resendVerification()).message });
    } catch (err) {
      setResend({ ok: false, text: err instanceof Error ? err.message : "Не удалось отправить письмо" });
    } finally {
      setResending(false);
    }
  }

  async function change(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setResult(null);
    try {
      setResult({ ok: true, text: (await api.changeEmail(email, password)).message });
      setEmail("");
      setPassword("");
    } catch (err) {
      setResult({ ok: false, text: err instanceof Error ? err.message : "Не удалось сменить адрес" });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Почта</CardTitle>
        <CardDescription>На неё приходит ссылка, если забыли пароль.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {user.email_verified ? (
          <p className="flex items-center gap-2 text-sm">
            <MailCheck className="size-4 shrink-0 text-success" />
            <span>
              <span className="font-medium">{user.email}</span> — подтверждена
            </span>
          </p>
        ) : (
          <div className="grid gap-2">
            <p className="flex items-start gap-2 text-sm">
              <MailWarning className="mt-0.5 size-4 shrink-0 text-warning-foreground" />
              <span>
                <span className="font-medium">{user.email}</span> ещё не подтверждена: перейдите по ссылке из письма, иначе
                восстановить пароль не получится.
              </span>
            </p>
            <Button variant="outline" className="justify-self-start" disabled={resending} onClick={sendAgain}>
              {resending ? "Отправляем..." : "Отправить письмо ещё раз"}
            </Button>
            {resend && <p className={resend.ok ? "text-sm text-success" : "text-sm text-destructive"}>{resend.text}</p>}
          </div>
        )}
        <Separator />
        <form className="grid gap-3" onSubmit={change}>
          <div className="grid gap-2">
            <Label htmlFor="email-new">Новый адрес</Label>
            <Input id="email-new" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="email-password">Пароль</Label>
            <Input
              id="email-password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          <p className="text-xs text-muted-foreground">
            На новый адрес придёт письмо; адрес сменится, когда вы перейдёте по ссылке.
          </p>
          {result && <p className={result.ok ? "text-sm text-success" : "text-sm text-destructive"}>{result.text}</p>}
          <Button size="lg" variant="outline" className="justify-self-start" disabled={busy}>
            {busy ? "Подождите..." : "Сменить адрес"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
