import md5 from "md5";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { useSync } from "@/offline/sync";

type UserAvatarProps = {
  email: string;
  name?: string;
  className?: string;
};

// The person's Gravatar, or their initial when it can't load, such as
// while offline (when it isn't even asked for). Decorative: their name is
// always shown next to it.
export default function UserAvatar({
  email,
  name,
  className,
}: UserAvatarProps) {
  const { offline } = useSync();
  const hash = md5(email.trim().toLowerCase());
  const initial = (name || email).trim().charAt(0).toUpperCase();

  return (
    <Avatar className={className} aria-hidden>
      {!offline && (
        <AvatarImage
          src={`https://www.gravatar.com/avatar/${hash}?d=retro`}
          alt=""
        />
      )}
      <AvatarFallback className="text-xs font-bold text-muted-foreground">
        {initial}
      </AvatarFallback>
    </Avatar>
  );
}
