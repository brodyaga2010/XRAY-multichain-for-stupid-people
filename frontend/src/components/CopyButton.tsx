import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { copyText } from "../utils";

export function CopyButton({ text, label = "Копировать" }: { text: string; label?: string }) {
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
    <button type="button" className="small ghost" onClick={copy}>
      {done ? <Check size={15} /> : <Copy size={15} />}
      <span>{done ? "Скопировано" : label}</span>
    </button>
  );
}
