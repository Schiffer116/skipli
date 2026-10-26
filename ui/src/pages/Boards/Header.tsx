import { useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  closestCenter,
  DndContext,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  horizontalListSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { LogOut, Monitor, Moon, Star, Sun, UsersRound } from "lucide-react";

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
import { useOffline } from "@/offline/network";
import { usePending } from "@/offline/pending";
import { apiFetch } from "@/utils/apiFetch";
import { SpaceKeyboardSensor } from "@/utils/keyboardSensor";
import { readTheme, setTheme, type Theme } from "@/utils/theme";

import { forgetMe, type Favorite, type Me } from "./loaders";

type HeaderProps = {
  me: Me;
  setMe: (me: Me) => void;
  favorites: Favorite[];
  orderFavorites: (favorites: Favorite[]) => void;
};

export default function Header({
  me,
  setMe,
  favorites,
  orderFavorites,
}: HeaderProps) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [confirmSignOut, setConfirmSignOut] = useState(false);
  const offline = useOffline();
  const unsaved = usePending().queued;
  const navigate = useNavigate();

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
                <DialogTitle>Account</DialogTitle>
                <DialogDescription>
                  Your name is shown to the members of your boards instead of
                  your email, {me.email}.
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
              <ThemePicker />
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
              You have {changes(unsaved)} waiting to sync. Signing out deletes
              your boards from this device, and those changes with them.
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

      {favorites.length > 0 && (
        <FavoriteBar
          me={me}
          favorites={favorites}
          orderFavorites={orderFavorites}
        />
      )}
    </header>
  );
}

type FavoriteBarProps = {
  me: Me;
  favorites: Favorite[];
  orderFavorites: (favorites: Favorite[]) => void;
};

// The links are the drag handles: a press that moves 5px starts a drag, so a
// plain click still navigates, and Space (not Enter) picks one up by keyboard.
function FavoriteBar({ me, favorites, orderFavorites }: FavoriteBarProps) {
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 5 } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    }),
    useSensor(SpaceKeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    orderFavorites(
      arrayMove(
        favorites,
        favorites.findIndex((board) => board.id === active.id),
        favorites.findIndex((board) => board.id === over.id),
      ),
    );
  };

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragEnd={onDragEnd}
    >
      <SortableContext
        items={favorites}
        strategy={horizontalListSortingStrategy}
      >
        <nav
          aria-label="Favorite boards"
          className="bg-nav text-nav-muted border-t border-nav-muted/20 h-9 px-4 flex items-center gap-1 overflow-x-auto text-sm"
        >
          {favorites.map((board) => (
            <FavoriteLink key={board.id} board={board} me={me} />
          ))}
        </nav>
      </SortableContext>
    </DndContext>
  );
}

function FavoriteLink({ board, me }: { board: Favorite; me: Me }) {
  const {
    attributes,
    listeners,
    transform,
    transition,
    isDragging,
    setNodeRef,
  } = useSortable({ id: board.id });

  return (
    <Link
      ref={setNodeRef}
      {...attributes}
      {...listeners}
      role="link"
      to={`/boards/${board.id}`}
      draggable={false}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
        zIndex: isDragging ? 10 : undefined,
      }}
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
  );
}

const themes = [
  { value: "system", label: "System", Icon: Monitor },
  { value: "light", label: "Light", Icon: Sun },
  { value: "dark", label: "Dark", Icon: Moon },
] satisfies { value: Theme; label: string; Icon: typeof Sun }[];

// Applies at once rather than on Save: it's remembered on this device,
// not on the account.
function ThemePicker() {
  const [theme, setChoice] = useState(readTheme);

  return (
    <div className="grid gap-2 mt-6">
      <p id="theme-label" className="text-sm leading-none font-medium">
        Appearance
      </p>
      <div
        role="radiogroup"
        aria-labelledby="theme-label"
        className="grid grid-cols-3 gap-1 rounded-lg border bg-background p-1"
      >
        {themes.map(({ value, label, Icon }) => (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={theme === value}
            onClick={() => {
              setTheme(value);
              setChoice(value);
            }}
            className="flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-checked:bg-card aria-checked:font-bold aria-checked:text-foreground aria-checked:shadow-sm dark:aria-checked:bg-accent dark:aria-checked:text-accent-foreground"
          >
            <Icon className="size-4" aria-hidden />
            {label}
          </button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        Remembered on this device. System follows your device's setting.
      </p>
    </div>
  );
}

// In the words the offline banner uses for the same changes.
const changes = (n: number) => `${n} ${n === 1 ? "change" : "changes"}`;
