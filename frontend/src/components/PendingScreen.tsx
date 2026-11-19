import { Clock3 } from "lucide-react";
import { User } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ThemeToggle } from "./ThemeToggle";

export function PendingScreen({ user, onLogout }: { user: User; onLogout: () => void }) {
  return (
    <main className="relative grid min-h-dvh place-items-center p-4">
      <div className="absolute top-3 right-3">
        <ThemeToggle />
      </div>
      <Card className="w-full max-w-md py-8">
        <CardContent className="grid justify-items-center gap-3 px-6 text-center">
          <span className="grid size-12 place-items-center rounded-full bg-warning/15 text-warning-foreground">
            <Clock3 className="size-6" />
          </span>
          <h1 className="text-xl font-semibold">Ждём подтверждения администратора</h1>
          <p className="text-sm text-muted-foreground">
            Аккаунт <span className="font-medium text-foreground">{user.email}</span> зарегистрирован. Как только
            администратор его подтвердит, здесь можно будет создавать ключи доступа.
          </p>
          <Button variant="outline" size="lg" className="mt-2" onClick={onLogout}>
            Выйти
          </Button>
        </CardContent>
      </Card>
    </main>
  );
}
