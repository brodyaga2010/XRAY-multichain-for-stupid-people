import { FormEvent, useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function CreateKeyForm({
  onCreate,
  submitLabel = "Создать",
  placeholder = "Название устройства (Телефон, Ноутбук)",
  autoFocus = false
}: {
  onCreate: (name: string) => Promise<void>;
  submitLabel?: string;
  placeholder?: string;
  autoFocus?: boolean;
}) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await onCreate(name.trim());
      setName("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось создать ключ");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="grid gap-2" onSubmit={submit}>
      <div className="flex gap-2 max-sm:flex-col">
        <Input
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder={placeholder}
          aria-label="Название устройства"
          minLength={1}
          maxLength={60}
          autoFocus={autoFocus}
          required
        />
        <Button size="lg" disabled={busy || !name.trim()}>
          <Plus />
          {busy ? "Создание..." : submitLabel}
        </Button>
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </form>
  );
}
