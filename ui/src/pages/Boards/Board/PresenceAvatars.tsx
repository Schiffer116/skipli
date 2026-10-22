import { useState } from "react";

import UserAvatar from "@/components/UserAvatar";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { Person } from "@/utils/socket";

// How many avatars show before the rest fold into "+N".
export const SHOWN = 4;

// The other people with the board open, overlapping like in Google Docs. A
// name shows on hover, or on tap where there is no hover.
export default function PresenceAvatars({ people }: { people: Person[] }) {
  if (people.length === 0) return null;
  const shown = people.slice(0, SHOWN);
  const more = people.slice(SHOWN);
  return (
    <ul
      aria-label="People viewing this board"
      className="flex shrink-0 items-center -space-x-2"
    >
      {shown.map((person) => (
        <li key={person.id}>
          <Named label={person.name}>
            <UserAvatar
              email={person.email}
              name={person.name}
              className="size-8 ring-2 ring-background"
            />
          </Named>
        </li>
      ))}
      {more.length > 0 && (
        <li>
          <Named label={more.map((person) => person.name).join(", ")}>
            <span
              aria-hidden
              className="flex size-8 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground ring-2 ring-background"
            >
              +{more.length}
            </span>
          </Named>
        </li>
      )}
    </ul>
  );
}

function Named({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          className="block rounded-full"
          onClick={() => setOpen((open) => !open)}
        >
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
