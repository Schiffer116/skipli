import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  closestCenter,
  DndContext,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type Modifier,
} from "@dnd-kit/core";
import {
  arrayMove,
  horizontalListSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { LogIn, LogOut, Star, UsersRound } from "lucide-react";

import ThemeMenu from "@/components/ThemeMenu";
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
import { GUEST } from "@/offline/guest";
import { signOut } from "@/offline/signOut";
import { useOffline } from "@/offline/network";
import { usePending } from "@/offline/pending";
import { apiFetch } from "@/utils/apiFetch";
import { SpaceKeyboardSensor } from "@/utils/keyboardSensor";

import { clampToBar } from "./clampToBar";
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

        <div className="flex items-center gap-1">
          <ThemeMenu />
          {me.id === GUEST.id ? (
            <Link
              to="/login"
              className="flex items-center gap-2 rounded px-2 py-1 text-sm font-bold text-nav-foreground hover:bg-nav-muted/15"
            >
              <LogIn className="size-4" aria-hidden />
              Sign in to save and share
            </Link>
          ) : (
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
                      Your name is shown to the members of your boards instead
                      of your email, {me.email}.
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
          )}
        </div>
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
  const barRef = useRef<HTMLElement>(null);
  // Set while a drag is on, and for the click that ends it. dnd-kit stops that
  // click's propagation, but the browser still follows the link, so the click
  // is cancelled at the window, where it arrives before dnd-kit sees it. A drag
  // that ends without a click (a keyboard drop, say) leaves the flag set, so
  // the next press or key clears it; Enter on a link is a click, so keys count.
  const dragging = useRef(false);

  useEffect(() => {
    const cancelClick = (event: MouseEvent) => {
      if (!dragging.current) return;
      event.preventDefault();
      dragging.current = false;
    };
    const clear = () => {
      dragging.current = false;
    };
    window.addEventListener("click", cancelClick, true);
    window.addEventListener("pointerdown", clear, true);
    window.addEventListener("keydown", clear, true);
    return () => {
      window.removeEventListener("click", cancelClick, true);
      window.removeEventListener("pointerdown", clear, true);
      window.removeEventListener("keydown", clear, true);
    };
  }, []);

  // A drag moves sideways along the bar only, and stays within its edges.
  const restrictToBarAxis: Modifier = ({ transform, draggingNodeRect }) => {
    const bar = barRef.current?.getBoundingClientRect();
    if (!bar || !draggingNodeRect) return { ...transform, y: 0 };
    return {
      ...transform,
      x: clampToBar(transform.x, bar, draggingNodeRect),
      y: 0,
    };
  };

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
      modifiers={[restrictToBarAxis]}
      collisionDetection={closestCenter}
      onDragStart={() => {
        dragging.current = true;
      }}
      onDragEnd={onDragEnd}
    >
      <SortableContext
        items={favorites}
        strategy={horizontalListSortingStrategy}
      >
        <nav
          ref={barRef}
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

// In the words the offline banner uses for the same changes.
const changes = (n: number) => `${n} ${n === 1 ? "change" : "changes"}`;
