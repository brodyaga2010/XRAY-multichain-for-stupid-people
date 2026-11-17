export function FilterChips<T extends string>({
  options,
  value,
  onChange,
  label
}: {
  options: Array<{ id: T; label: string }>;
  value: T;
  onChange: (value: T) => void;
  label?: string;
}) {
  return (
    <div className="chips" role="group" aria-label={label}>
      {options.map((item) => (
        <button
          key={item.id}
          type="button"
          className={`chip ${value === item.id ? "active" : ""}`}
          aria-pressed={value === item.id}
          onClick={() => onChange(item.id)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
