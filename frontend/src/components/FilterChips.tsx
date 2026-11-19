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
    <Tabs value={value} onValueChange={(next) => onChange(next as T)} className={cn("min-w-0", className)}>
      <TabsList aria-label={label} className={cn("h-9 w-full max-w-full scrollbar-none overflow-x-auto", !compact && "sm:w-fit")}>
        {options.map((item) => (
          <TabsTrigger key={item.id} value={item.id} className={compact ? "px-1 text-xs" : "px-1.5 text-xs sm:px-3 sm:text-sm"}>
            {item.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}
