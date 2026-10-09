import md5 from "md5";

import { Avatar, AvatarImage } from "@/components/ui/avatar";

type UserAvatarProps = {
  email: string;
  className?: string;
};

export default function UserAvatar({ email, className }: UserAvatarProps) {
  const hash = md5(email.trim().toLowerCase());
  return (
    <Avatar className={className}>
      <AvatarImage
        src={`https://www.gravatar.com/avatar/${hash}?d=retro`}
        alt=""
      />
    </Avatar>
  );
}
