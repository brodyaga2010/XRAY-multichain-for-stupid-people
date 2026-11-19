import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";

/** Переключатель фильтра (сегменты). */
export function FilterChips<T extends string>({
  options,
  value,
  onChange,
  label,
  compact = false,
  className
}: {
  options: Array<{ id: T; label: string }>;
  value: T;
  onChange: (value: T) => void;
  label?: string;
  /** Мелкий шрифт и растяжка на всю ширину — для узких колонок. */
  compact?: boolean;
  className?: string;
}) {
  return (
    <Tabs value={value} onValueChange={(next) => onChange(next as T)} className={className}>
      <TabsList aria-label={label} className={cn("h-9 max-w-full overflow-x-auto", compact && "w-full")}>
        {options.map((item) => (
          <TabsTrigger key={item.id} value={item.id} className={compact ? "px-1 text-xs" : "px-3"}>
            {item.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}
