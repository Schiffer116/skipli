import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { LogOut, Star, UsersRound } from "lucide-react";

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
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { signOut } from "@/offline/signOut";
import { useSync } from "@/offline/sync";
import { apiFetch } from "@/utils/apiFetch";

import { forgetMe, type Favorite, type Me } from "./loaders";

type HeaderProps = {
  me: Me;
  setMe: (me: Me) => void;
  favorites: Favorite[];
};

export default function Header({ me, setMe, favorites }: HeaderProps) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [confirmSignOut, setConfirmSignOut] = useState(false);
  const { offline, pendingCount, rejectedCount } = useSync();
  const navigate = useNavigate();
  const unsaved = pendingCount + rejectedCount;

  const finishSignOut = async () => {
    await signOut();
    forgetMe();
    navigate("/login", { replace: true });
  };

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
                name={me.name}
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
                {offline && (
                  <p className="text-sm text-muted-foreground">
                    Changing your name needs a connection.
                  </p>
                )}
              </div>
              <DialogFooter className="mt-4">
                <Button
                  variant="ghost"
                  type="button"
                  className="sm:mr-auto"
                  onClick={() => {
                    if (unsaved === 0) {
                      void finishSignOut();
                      return;
                    }
                    // One dialog at a time: the confirmation replaces this one.
                    setOpen(false);
                    setConfirmSignOut(true);
                  }}
                >
                  <LogOut />
                  Sign out
                </Button>
                <DialogClose asChild>
                  <Button variant="outline" type="button">
                    Cancel
                  </Button>
                </DialogClose>
                <Button type="submit" disabled={offline}>
                  Save
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <AlertDialog open={confirmSignOut} onOpenChange={setConfirmSignOut}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Sign out and discard unsaved changes?
            </AlertDialogTitle>
            <AlertDialogDescription>
              You have {unsavedSummary(pendingCount, rejectedCount)}. Signing
              out deletes your boards from this device, and those changes with
              them.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Stay signed in</AlertDialogCancel>
            <AlertDialogAction onClick={finishSignOut}>
              Sign out and discard
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

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

const changes = (n: number) => `${n} ${n === 1 ? "change" : "changes"}`;

// In the words the offline banner uses for the same changes.
function unsavedSummary(pending: number, rejected: number) {
  const waiting = `${changes(pending)} waiting to sync`;
  const refused = `${changes(rejected)} that couldn't be saved`;
  if (rejected === 0) return waiting;
  if (pending === 0) return refused;
  return `${waiting} and ${rejected} that couldn't be saved`;
}
