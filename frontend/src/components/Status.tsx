const labels: Record<string, string> = {
  active: "Активен",
  suspended: "Отключён",
  revoked: "Отозван",
  pending: "Ждёт подтверждения",
  approved: "Подтверждён"
};

export function Status({ status, kind = "key" }: { status: string; kind?: "key" | "user" }) {
  const label = kind === "user" && status === "suspended" ? "Заблокирован" : labels[status] ?? status;
  return <span className={`status ${status}`}>{label}</span>;
}
