import { ChevronDown, QrCode } from "lucide-react";
import { ReactNode, useState } from "react";
import { KeyLink, VpnKey } from "@/api";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useMediaQuery } from "@/hooks";
import { cn } from "@/lib/utils";
import { keyTitle } from "@/utils";
import { CopyButton } from "./CopyButton";
import { QrImage } from "./QrImage";

const transportLabels: Record<string, string> = {
  tcp: "Основное (TCP)",
  grpc: "Запасное (gRPC)",
  xhttp: "Запасное (XHTTP)"
};

function LinkBlock({ link, primary, alwaysQr }: { link: KeyLink; primary: boolean; alwaysQr: boolean }) {
  const [qrOpen, setQrOpen] = useState(false);
  const qrShown = alwaysQr || qrOpen;

  return (
    <section className={cn("grid gap-3 rounded-lg border p-3", primary && "border-primary/25 bg-accent/40")}>
      <h3 className="text-sm font-medium">{transportLabels[link.transport] ?? link.label}</h3>
      <code className="truncate rounded-md border bg-background px-3 py-2 font-mono text-xs">{link.uri}</code>
      <div className="flex flex-wrap gap-2">
        <CopyButton
          text={link.uri}
          label="Скопировать ключ"
          size="lg"
          variant={primary ? "default" : "outline"}
          className={cn(primary && "flex-1 sm:flex-none")}
        />
        {!alwaysQr && (
          <Button type="button" variant="outline" size="lg" onClick={() => setQrOpen((current) => !current)}>
            <QrCode />
            {qrOpen ? "Скрыть QR" : "Показать QR"}
          </Button>
        )}
      </div>
      {qrShown && <QrImage value={link.uri} />}
    </section>
  );
}

function Fold({ title, children }: { title: string; children: ReactNode }) {
  return (
    <details className="group rounded-lg border">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-2 px-3 py-2.5 text-sm font-medium [&::-webkit-details-marker]:hidden">
        {title}
        <ChevronDown className="size-4 text-muted-foreground transition-transform group-open:rotate-180" />
      </summary>
      <div className="grid gap-3 border-t p-3">{children}</div>
    </details>
  );
}

export function KeyModal({ keyItem, onClose }: { keyItem: VpnKey; onClose: () => void }) {
  const isPhone = useMediaQuery("(max-width: 640px)");
  const disabled = keyItem.status === "suspended" || keyItem.status === "expired";
  const primaryLink = keyItem.links.find((link) => link.transport === "tcp") ?? keyItem.links[0];
  const fallbackLinks = keyItem.links.filter((link) => link !== primaryLink);

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className={cn(
          "max-h-[90dvh] gap-4 overflow-y-auto p-5 sm:max-w-lg",
          // на телефоне — панель снизу экрана
          "max-sm:top-auto max-sm:bottom-0 max-sm:max-w-full max-sm:translate-y-0 max-sm:rounded-b-none max-sm:pb-[calc(1.25rem+env(safe-area-inset-bottom))]"
        )}
      >
        <DialogHeader className="pr-8">
          <DialogDescription>Подключение</DialogDescription>
          <DialogTitle className="truncate text-lg" title={keyTitle(keyItem)}>
            {keyTitle(keyItem)}
          </DialogTitle>
        </DialogHeader>
        {disabled ? (
          <p className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
            {keyItem.status === "expired"
              ? "Подписка закончилась — ключ снова заработает после продления"
              : "Ключ отключён администратором"}
          </p>
        ) : (
          <>
            {primaryLink ? (
              <LinkBlock link={primaryLink} primary alwaysQr={!isPhone} />
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">Ссылок нет</p>
            )}
            {fallbackLinks.length > 0 && (
              <Fold title="Запасные варианты (gRPC, XHTTP)">
                {fallbackLinks.map((link) => (
                  <LinkBlock key={link.transport} link={link} primary={false} alwaysQr={false} />
                ))}
              </Fold>
            )}
            <Fold title="Как подключиться">
              <ol className="grid list-decimal gap-1.5 pl-5 text-sm text-muted-foreground marker:text-foreground">
                <li>
                  Установите приложение. Android: v2rayNG, Hiddify. iPhone: Streisand, Hiddify, v2RayTun. Windows и macOS:
                  Hiddify, v2rayN. AmneziaVPN тоже подходит.
                </li>
                <li>Скопируйте ссылку «Основное (TCP)».</li>
                <li>В приложении нажмите «Добавить из буфера» или «Вставить ключ».</li>
                <li>Подключитесь.</li>
              </ol>
              <p className="text-sm text-muted-foreground">Если не работает — попробуйте запасные ссылки.</p>
            </Fold>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
