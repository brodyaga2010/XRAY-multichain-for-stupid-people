import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

/** Кружок с первой буквой почты. */
export function UserAvatar({ email, size, className }: { email: string; size?: "sm" | "default" | "lg"; className?: string }) {
  return (
    <Avatar size={size} className={className}>
      <AvatarFallback className="bg-accent font-semibold text-accent-foreground">
        {email.slice(0, 1).toUpperCase()}
      </AvatarFallback>
    </Avatar>
  );
}
