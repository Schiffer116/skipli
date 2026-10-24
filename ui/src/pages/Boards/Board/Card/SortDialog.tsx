import { useContext, useRef, useState } from "react";
import { ArrowDownWideNarrow } from "lucide-react";
import { useParams } from "react-router";
import { toast } from "sonner";

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
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SortError, sortCard, sortMoves } from "@/board/sort";
import { useOffline } from "@/offline/network";

import type { TaskType } from "./Task/Task";
import { BoardContext } from "../BoardContext";

const RECENT_KEY = "skipli:sort-criteria";
const RECENT_KEPT = 5;
const UNDO_FOR = 10_000;

function readRecent(): string[] {
  try {
    const recent = JSON.parse(localStorage.getItem(RECENT_KEY) ?? "[]");
    return Array.isArray(recent) ? recent : [];
  } catch {
    return [];
  }
}

function remember(criterion: string) {
  const recent = [criterion, ...readRecent().filter((c) => c !== criterion)];
  try {
    localStorage.setItem(
      RECENT_KEY,
      JSON.stringify(recent.slice(0, RECENT_KEPT)),
    );
  } catch {
    // Not kept on this device; sorting works all the same.
  }
}

// Sort: reorders a card's tasks by a criterion the person types, as Jev
// judges it. The server only answers with the order; the moves are
// ordinary Board changes, so they show at once, wait as Pending changes,
// reach teammates, and can be undone from the toast.
export default function SortDialog({
  id,
  tasks,
}: {
  id: string;
  tasks: TaskType[];
}) {
  const boardId = useParams().boardId!;
  const { make, setCreateTaskFormId, setShowCreateCardForm } =
    useContext(BoardContext)!;
  const offline = useOffline();

  const [open, setOpen] = useState(false);
  const [sorting, setSorting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // The card's tasks as they are now, for moves worked out after an await
  // or from the toast, by when the card may have changed.
  const tasksRef = useRef(tasks);
  tasksRef.current = tasks;
  const ids = () => tasksRef.current.map((task) => task.id);

  const moveTo = (target: string[]) => {
    const moves = sortMoves(boardId, id, ids(), target);
    moves.forEach((move) => void make(move));
    return moves.length;
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const criterion = (
      new FormData(e.currentTarget).get("criterion") as string
    ).trim();
    if (!criterion) return;

    setSorting(true);
    setError(null);
    try {
      const { order, model } = await sortCard(boardId, id, criterion);
      remember(criterion);
      const before = ids();
      const moved = moveTo(order);
      setOpen(false);
      const fake = model === "fake" ? " (fake judge)" : "";
      if (moved === 0) {
        toast(`Already sorted by "${criterion}"${fake}`);
      } else {
        toast(`Sorted by "${criterion}"${fake}`, {
          action: { label: "Undo", onClick: () => moveTo(before) },
          // Long enough to look the new order over before undoing it.
          duration: UNDO_FOR,
        });
      }
    } catch (err) {
      setError(
        err instanceof SortError ? err.message : "Couldn't sort this card.",
      );
    } finally {
      setSorting(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(open) => {
        setOpen(open);
        setError(null);
      }}
    >
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Sort tasks"
          title={offline ? "Sorting needs a connection" : "Sort tasks"}
          disabled={offline}
          onClick={(e) => {
            e.stopPropagation();
            setCreateTaskFormId(null);
            setShowCreateCardForm(false);
          }}
        >
          <ArrowDownWideNarrow />
        </Button>
      </DialogTrigger>
      {open && (
        <DialogContent
          className="sm:max-w-[425px]"
          onClick={(e) => e.stopPropagation()}
          // As in EditCardDialog: keep a text selection from starting a
          // drag of the card behind.
          onMouseDown={(e) => e.stopPropagation()}
        >
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>Sort tasks</DialogTitle>
              <DialogDescription>
                Task names and descriptions are sent to TypeSafe's Jev to sort.
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-3 mt-4">
              <Label htmlFor="sort-criterion">Sort by</Label>
              <Input
                id="sort-criterion"
                name="criterion"
                placeholder="most urgent first"
                maxLength={200}
                list="sort-criteria"
                autoComplete="off"
                required
                disabled={sorting}
              />
              <datalist id="sort-criteria">
                {readRecent().map((criterion) => (
                  <option key={criterion} value={criterion} />
                ))}
              </datalist>
              {error && (
                <p role="alert" className="text-sm text-destructive">
                  {error}
                </p>
              )}
            </div>
            <DialogFooter className="mt-4 flex-row justify-end">
              <DialogClose asChild>
                <Button type="button" variant="outline">
                  Cancel
                </Button>
              </DialogClose>
              <Button type="submit" disabled={sorting || offline}>
                {sorting ? "Sorting…" : "Sort"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      )}
    </Dialog>
  );
}
