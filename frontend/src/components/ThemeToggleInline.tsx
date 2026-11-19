import { Monitor, Moon, Sun } from "lucide-react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ThemeChoice, useTheme } from "@/lib/theme";

/** Переключатель темы сегментами — для страницы профиля. */
export function ThemeToggleInline() {
  const [choice, setChoice] = useTheme();
  return (
    <Tabs value={choice} onValueChange={(value) => setChoice(value as ThemeChoice)}>
      <TabsList className="h-9">
        <TabsTrigger value="light" aria-label="Светлая" title="Светлая" className="px-2.5">
          <Sun />
        </TabsTrigger>
        <TabsTrigger value="dark" aria-label="Тёмная" title="Тёмная" className="px-2.5">
          <Moon />
        </TabsTrigger>
        <TabsTrigger value="system" aria-label="Как в системе" title="Как в системе" className="px-2.5">
          <Monitor />
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}
