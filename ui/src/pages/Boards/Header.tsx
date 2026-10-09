import { Link } from "react-router";
import { Star } from "lucide-react";
import UserAvatar from "@/components/UserAvatar";

import type { Favorite } from "./loaders";

type HeaderProps = {
  email: string;
  favorites: Favorite[];
};

export default function Header({ email, favorites }: HeaderProps) {
  return (
    <header className="sticky top-0 z-20">
      <div className="bg-nav text-nav-foreground h-12 px-4 w-full flex items-center gap-2 justify-between">
        <Link to="/boards" className="flex items-center gap-2">
          <img src="/skipli.png" alt="Logo" width={28} height={28} />
          <p className="font-bold text-base">Skipli</p>
        </Link>

        <div className="flex items-center gap-3 text-sm text-nav-muted">
          <p>{email}</p>
          <UserAvatar
            email={email}
            className="size-7 ring-1 ring-nav-muted/40"
          />
        </div>
      </div>

      <nav
        aria-label="Favorite boards"
        className="bg-nav text-nav-muted border-t border-nav-muted/20 h-9 px-4 flex items-center gap-1 overflow-x-auto text-sm"
      >
        {favorites.map((board) => (
          <Link
            key={board.id}
            to={`/boards/${board.id}`}
            className="flex shrink-0 items-center gap-1.5 rounded px-2 py-1 hover:bg-nav-muted/15 hover:text-nav-foreground"
          >
            <Star
              className="size-3.5 fill-current text-amber-400"
              aria-hidden
            />
            {board.name}
          </Link>
        ))}
      </nav>
    </header>
  );
}
