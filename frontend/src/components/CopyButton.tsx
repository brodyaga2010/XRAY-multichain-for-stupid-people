import { Check, Copy } from "lucide-react";
import { ComponentProps, useState } from "react";
import { Button } from "@/components/ui/button";
import { copyText } from "@/utils";

export function CopyButton({
  text,
  label = "Копировать",
  ...props
}: { text: string; label?: string } & Omit<ComponentProps<typeof Button>, "onClick" | "children">) {
  const [done, setDone] = useState(false);

  async function copy() {
    try {
      await copyText(text);
      setDone(true);
      setTimeout(() => setDone(false), 1800);
    } catch {
      window.prompt("Скопируйте вручную:", text);
    }
  }

  return (
    <Button type="button" onClick={copy} disabled={!text} {...props}>
      {done ? <Check /> : <Copy />}
      {done ? "Скопировано" : label}
    </Button>
  );
}
