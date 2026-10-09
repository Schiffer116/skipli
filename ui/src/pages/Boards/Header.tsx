import { useState } from "react";
import { Link } from "react-router";
import { Star, UsersRound } from "lucide-react";

import UserAvatar from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiFetch } from "@/utils/apiFetch";

import type { Favorite, Me } from "./loaders";

type HeaderProps = {
  me: Me;
  setMe: (me: Me) => void;
  favorites: Favorite[];
};

export default function Header({ me, setMe, favorites }: HeaderProps) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");

  const saveName = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const name = (new FormData(e.currentTarget).get("name") as string).trim();

    const res = await apiFetch("/api/me", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    });
    if (!res.ok) {
      setError("Use 1 to 50 characters.");
      return;
    }

    setMe(await res.json());
    setOpen(false);
  };

  return (
    <header className="sticky top-0 z-20">
      <div className="bg-nav text-nav-foreground h-12 px-4 w-full flex items-center gap-2 justify-between">
        <Link to="/boards" className="flex items-center gap-2">
          <img src="/skipli.png" alt="Logo" width={28} height={28} />
          <p className="font-bold text-base">Skipli</p>
        </Link>

        <Dialog
          open={open}
          onOpenChange={(open) => {
            setOpen(open);
            setError("");
          }}
        >
          <DialogTrigger asChild>
            <button
              type="button"
              title={me.email}
              className="flex items-center gap-3 text-sm text-nav-muted rounded px-2 py-1 hover:bg-nav-muted/15 hover:text-nav-foreground"
            >
              {me.name}
              <UserAvatar
                email={me.email}
                className="size-7 ring-1 ring-nav-muted/40"
              />
            </button>
          </DialogTrigger>
          <DialogContent className="sm:max-w-[425px]">
            <form onSubmit={saveName} autoComplete="off">
              <DialogHeader className="mb-4">
                <DialogTitle>Display name</DialogTitle>
                <DialogDescription>
                  Shown to the members of your boards instead of your email,{" "}
                  {me.email}.
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-2">
                <Label htmlFor="display-name">Name</Label>
                <Input
                  id="display-name"
                  name="name"
                  defaultValue={me.name}
                  maxLength={50}
                  required
                  aria-invalid={error !== ""}
                />
                {error && (
                  <p role="alert" className="text-sm text-destructive">
                    {error}
                  </p>
                )}
              </div>
              <DialogFooter className="mt-4">
                <DialogClose asChild>
                  <Button variant="outline" type="button">
                    Cancel
                  </Button>
                </DialogClose>
                <Button type="submit">Save</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
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
            {board.owner === me.id ? (
              <Star
                className="size-3.5 fill-current text-amber-400"
                aria-label="Your board"
              />
            ) : (
              <UsersRound
                className="size-3.5 text-sky-400"
                aria-label="Shared with you"
              />
            )}
            {board.name}
          </Link>
        ))}
      </nav>
    </header>
  );
}
