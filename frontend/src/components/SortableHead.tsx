import { ArrowDown, ArrowUp, ChevronsUpDown } from "lucide-react";
import type { ReactNode } from "react";
import { TableHead } from "@/components/ui/table";
import type { SortState } from "@/lib/sort";
import { cn } from "@/lib/utils";

/** Заголовок колонки, по которому можно отсортировать таблицу. */
export function SortableHead<K extends string>({
  column,
  sort,
  onSort,
  children,
  className,
  align = "left"
}: {
  column: K;
  sort: SortState<K>;
  onSort: (column: K) => void;
  children: ReactNode;
  className?: string;
  align?: "left" | "right";
}) {
  const active = sort.key === column;
  const Icon = !active ? ChevronsUpDown : sort.dir === "asc" ? ArrowUp : ArrowDown;
  return (
    <TableHead
      className={cn(align === "right" && "text-right", className)}
      aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}
    >
      <button
        type="button"
        onClick={() => onSort(column)}
        className={cn(
          "-mx-1.5 inline-flex items-center gap-1 rounded-md px-1.5 py-1 transition-colors hover:bg-muted hover:text-foreground",
          active && "text-foreground"
        )}
      >
        {children}
        <Icon className={cn("size-3.5", !active && "opacity-40")} />
      </button>
    </TableHead>
  );
}
