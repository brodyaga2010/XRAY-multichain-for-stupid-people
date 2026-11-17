import { FormEvent, useState } from "react";
import { Plus } from "lucide-react";

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
    <form className="inline-form" onSubmit={submit}>
      <input
        value={name}
        onChange={(event) => setName(event.target.value)}
        placeholder={placeholder}
        aria-label="Название устройства"
        minLength={1}
        maxLength={60}
        autoFocus={autoFocus}
        required
      />
      <button className="primary" disabled={busy || !name.trim()}>
        <Plus size={17} /> {busy ? "Создание..." : submitLabel}
      </button>
      {error && <p className="error inline-form-error">{error}</p>}
    </form>
  );
}
