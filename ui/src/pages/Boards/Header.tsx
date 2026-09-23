import { Link, useLoaderData } from "react-router";
import type { boardsLoader } from "./Boards";
import { Avatar, AvatarImage } from "@/components/ui/avatar";

import md5 from "md5";

export default function Header() {
  const { email } = useLoaderData<typeof boardsLoader>();

  const hash = md5(email);
  const avatarUrl = `https://www.gravatar.com/avatar/${hash}?d=identicon`;

  return (
    <header className="sticky top-0 z-20">
      <div className="bg-nav text-nav-foreground h-12 px-4 w-screen flex items-center gap-2 justify-between">
        <Link to="/boards" className="flex items-center gap-2">
          <img src="/skipli.png" alt="Logo" width={28} height={28} />
          <p className="font-bold text-base">Skipli</p>
        </Link>

        <div className="flex items-center gap-3 text-sm text-nav-muted">
          <p>{email}</p>
          <Avatar className="size-7 ring-1 ring-nav-muted/40">
            <AvatarImage src={avatarUrl} alt={email} />
          </Avatar>
        </div>
      </div>
    </header>
  );
}
