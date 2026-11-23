import { Check, Copy } from "lucide-react";
import { ComponentProps, useState } from "react";
import { Button } from "@/components/ui/button";
import { copyText } from "@/utils";

export function CopyButton({
  text,
  label = "Копировать",
  iconOnly = false,
  ...props
}: { text: string; label?: string; iconOnly?: boolean } & Omit<
  ComponentProps<typeof Button>,
  "onClick" | "children"
>) {
  const [done, setDone] = useState(false);
  const caption = done ? "Скопировано" : label;

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
    <Button
      type="button"
      onClick={copy}
      disabled={!text}
      {...(iconOnly && { "aria-label": caption, title: caption })}
      {...props}
    >
      {done ? <Check /> : <Copy />}
      {!iconOnly && caption}
    </Button>
  );
}
