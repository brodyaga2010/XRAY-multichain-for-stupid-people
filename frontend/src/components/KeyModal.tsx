import { ChevronDown, QrCode } from "lucide-react";
import { ReactNode, useState } from "react";
import { KeyLink, VpnKey } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useMediaQuery } from "@/hooks";
import { keyPreview } from "@/lib/keyLink";
import { cn } from "@/lib/utils";
import { keyTitle } from "@/utils";
import { CopyButton } from "./CopyButton";
import { QrImage } from "./QrImage";

const transportNames: Record<string, string> = { tcp: "TCP", grpc: "gRPC", xhttp: "XHTTP" };
const fallbackHints: Record<string, string> = {
  grpc: "Если основной не подключается",
  xhttp: "Если не работают оба других"
};

const apps = [
  { platform: "Android", names: ["v2rayNG", "Hiddify"] },
  { platform: "iPhone", names: ["Streisand", "Hiddify", "v2RayTun"] },
  { platform: "Windows, macOS", names: ["Hiddify", "v2rayN"] }
];

function transportName(link: KeyLink) {
  return transportNames[link.transport] ?? link.label;
}

/** Основной ключ: на компьютере QR слева и кнопка справа, на телефоне — кнопка во всю ширину и QR по запросу. */
function PrimaryKey({ link, isPhone }: { link: KeyLink; isPhone: boolean }) {
  const [qrOpen, setQrOpen] = useState(false);

  return (
    <section className="rounded-xl border border-primary/25 bg-accent/40 p-4">
      <div className="flex items-center gap-4">
        {!isPhone && <QrImage value={link.uri} size={168} />}
        <div className="grid min-w-0 flex-1 gap-3">
          <div className="grid gap-1">
            <div className="flex items-center gap-2">
              <h3 className="font-medium">Основной ключ</h3>
              <Badge variant="secondary">{transportName(link)}</Badge>
            </div>
            <p className="text-sm text-muted-foreground">
              {isPhone
                ? "Скопируйте ключ и вставьте его в приложение"
                : "Отсканируйте QR камерой телефона в приложении или скопируйте ключ"}
            </p>
          </div>
          <CopyButton text={link.uri} label="Скопировать ключ" className="h-11 w-full text-base" />
          {isPhone && (
            <Button
              type="button"
              variant="ghost"
              className="h-10 w-full"
              aria-expanded={qrOpen}
              onClick={() => setQrOpen((current) => !current)}
            >
              <QrCode />
              {qrOpen ? "Скрыть QR-код" : "Показать QR-код"}
            </Button>
          )}
          <code
            className="truncate text-center font-mono text-xs text-muted-foreground select-all sm:text-left"
            title={link.uri}
          >
            {keyPreview(link.uri)}
          </code>
        </div>
      </div>
      {isPhone && qrOpen && (
        <div className="mt-3 grid">
          <QrImage value={link.uri} />
        </div>
      )}
    </section>
  );
}

/** Запасной ключ — строка списка: название, когда пригодится, и кнопки-иконки. */
function FallbackKey({ link }: { link: KeyLink }) {
  const [qrOpen, setQrOpen] = useState(false);
  const name = transportName(link);

  return (
    <li className="grid gap-3 py-3 first:pt-0 last:pb-0">
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">{name}</p>
          <p className="truncate text-xs text-muted-foreground">{fallbackHints[link.transport] ?? "Запасной ключ"}</p>
        </div>
        <CopyButton text={link.uri} label={`Скопировать ключ ${name}`} iconOnly variant="outline" className="size-10" />
        <Button
          type="button"
          variant={qrOpen ? "secondary" : "outline"}
          className="size-10"
          aria-label={`QR-код ключа ${name}`}
          title={qrOpen ? "Скрыть QR-код" : "Показать QR-код"}
          aria-pressed={qrOpen}
          onClick={() => setQrOpen((current) => !current)}
        >
          <QrCode />
        </Button>
      </div>
      {qrOpen && <QrImage value={link.uri} />}
    </li>
  );
}

function Fold({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  return (
    <details className="group rounded-xl border">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 [&::-webkit-details-marker]:hidden">
        <span className="grid gap-0.5">
          <span className="text-sm font-medium">{title}</span>
          {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
        </span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
      </summary>
      <div className="grid gap-3 border-t px-4 py-3">{children}</div>
    </details>
  );
}

function Steps() {
  const steps: ReactNode[] = [
    <>
      Установите приложение:
      <span className="mt-1.5 grid gap-1">
        {apps.map((app) => (
          <span key={app.platform} className="flex flex-wrap items-center gap-1">
            <span className="mr-1 text-foreground">{app.platform}</span>
            {app.names.map((name) => (
              <Badge key={name} variant="outline" className="font-normal">
                {name}
              </Badge>
            ))}
          </span>
        ))}
      </span>
    </>,
    "Скопируйте основной ключ или отсканируйте QR-код в приложении.",
    "В приложении нажмите «Добавить из буфера» или «Вставить ключ».",
    "Подключитесь. Если не выходит — попробуйте запасные ключи."
  ];

  return (
    <ol className="grid gap-3 text-sm text-muted-foreground">
      {steps.map((step, index) => (
        <li key={index} className="flex gap-3">
          <span className="grid size-6 shrink-0 place-items-center rounded-full bg-muted text-xs font-medium text-foreground">
            {index + 1}
          </span>
          <span className="min-w-0 pt-0.5">{step}</span>
        </li>
      ))}
    </ol>
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
          "max-h-[90dvh] gap-3 overflow-y-auto p-5 sm:max-w-xl",
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
              <PrimaryKey link={primaryLink} isPhone={isPhone} />
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">Ссылок нет</p>
            )}
            {fallbackLinks.length > 0 && (
              <Fold title="Запасные ключи" hint={fallbackLinks.map(transportName).join(", ")}>
                <ul className="grid divide-y">
                  {fallbackLinks.map((link) => (
                    <FallbackKey key={link.transport} link={link} />
                  ))}
                </ul>
              </Fold>
            )}
            <Fold title="Как подключиться">
              <Steps />
            </Fold>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
