import { FormEvent, useState } from "react";
import { Plus, X } from "lucide-react";
import { User } from "../api";

export function CreateKeyForm({
  user,
  activeCount,
  compact = false,
  onClose,
  onCreate
}: {
  user: User;
  activeCount?: number;
  compact?: boolean;
  onClose?: () => void;
  onCreate: (name: string) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const limitReached = activeCount !== undefined && activeCount >= user.key_limit;

  async function submit(event: FormEvent) {
    event.preventDefault();
    await onCreate(name.trim());
    setName("");
  }

  return (
    <form className={compact ? "inline-create" : "tool-panel"} onSubmit={submit}>
      {!compact && (
        <div className="panel-heading">
          <h2>Создать ключ</h2>
          {activeCount !== undefined && (
            <span>
              {activeCount}/{user.key_limit} активных
            </span>
          )}
        </div>
      )}
      {compact && (
        <div className="inline-create-head">
          <div>
            <h3>Создать ключ</h3>
            {activeCount !== undefined && (
              <span>
                {activeCount}/{user.key_limit} активных
              </span>
            )}
          </div>
          {onClose && (
            <button className="icon-button" type="button" onClick={onClose} aria-label="Закрыть форму создания ключа">
              <X size={16} />
            </button>
          )}
        </div>
      )}
      <input
        value={name}
        onChange={(event) => setName(event.target.value)}
        placeholder="Название устройства (Телефон, Ноутбук)"
        minLength={1}
        maxLength={60}
        required
      />
      <button className="primary" disabled={limitReached}>
        <Plus size={17} /> Создать ключ
      </button>
    </form>
  );
}
