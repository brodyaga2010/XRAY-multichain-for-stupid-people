import { FormEvent, useState } from "react";
import { Plus, X } from "lucide-react";
import { KeyType, User } from "../api";

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
  onCreate: (payload: { name: string; type: KeyType; sni?: string }) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [type, setType] = useState<KeyType>("tcp_reality");
  const [sni, setSni] = useState("");
  const limitReached = activeCount !== undefined && activeCount >= user.key_limit;

  async function submit(event: FormEvent) {
    event.preventDefault();
    await onCreate({ name, type, sni: sni || undefined });
    setName("");
    setSni("");
  }

  return (
    <form className={compact ? "inline-create" : "tool-panel"} onSubmit={submit}>
      {!compact && (
        <div className="panel-heading">
          <h2>Create key</h2>
          {activeCount !== undefined && (
            <span>
              {activeCount}/{user.key_limit} active
            </span>
          )}
        </div>
      )}
      {compact && (
        <div className="inline-create-head">
          <div>
            <h3>Create key</h3>
            {activeCount !== undefined && (
              <span>
                {activeCount}/{user.key_limit} active
              </span>
            )}
          </div>
          {onClose && (
            <button className="icon-button" type="button" onClick={onClose} aria-label="Close create key form">
              <X size={16} />
            </button>
          )}
        </div>
      )}
      <input value={name} onChange={(event) => setName(event.target.value)} placeholder="Device name" required />
      <select value={type} onChange={(event) => setType(event.target.value as KeyType)}>
        <option value="tcp_reality">TCP REALITY</option>
        <option value="xhttp_reality">XHTTP REALITY</option>
      </select>
      <input value={sni} onChange={(event) => setSni(event.target.value)} placeholder="Default SNI" />
      <button className="primary" disabled={limitReached}>
        <Plus size={17} /> Create key
      </button>
    </form>
  );
}
