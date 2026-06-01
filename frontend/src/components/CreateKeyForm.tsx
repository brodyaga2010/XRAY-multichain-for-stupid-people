import { FormEvent, useState } from "react";
import { Plus } from "lucide-react";
import { KeyType, User } from "../api";

export function CreateKeyForm({
  user,
  activeCount,
  compact = false,
  onCreate
}: {
  user: User;
  activeCount?: number;
  compact?: boolean;
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
