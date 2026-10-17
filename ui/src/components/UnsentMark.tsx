import { CloudOff } from "lucide-react";

// Marks a board, card or task with changes the server hasn't accepted yet.
export default function UnsentMark({ className = "" }: { className?: string }) {
  return (
    <span
      title="Not synced yet"
      className={`inline-flex shrink-0 text-muted-foreground ${className}`}
    >
      <CloudOff className="size-4" aria-label="Not synced yet" />
    </span>
  );
}
