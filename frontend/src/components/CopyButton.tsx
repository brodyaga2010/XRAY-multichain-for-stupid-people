import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { copyText } from "../utils";

export function CopyButton({
  text,
  label = "Копировать",
  className = "small ghost"
}: {
  text: string;
  label?: string;
  className?: string;
}) {
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
    <button type="button" className={className} onClick={copy} disabled={!text}>
      {done ? <Check size={16} /> : <Copy size={16} />}
      <span>{done ? "Скопировано" : label}</span>
    </button>
  );
}
