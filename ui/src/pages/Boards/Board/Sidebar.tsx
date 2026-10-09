import UserAvatar from "@/components/UserAvatar";
import {
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  SquareKanban,
  User,
} from "lucide-react";
import { useEffect, useState } from "react";
import { useLoaderData, useParams } from "react-router";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  DialogClose,
  DialogDescription,
  DialogTitle,
} from "@radix-ui/react-dialog";
import type { boardViewLoader } from "../loaders";
import { apiFetch } from "@/utils/apiFetch";

// Per-browser preference; storage can be unavailable (private mode etc.).
const COLLAPSED_KEY = "skipli:membersCollapsed";

function readCollapsed() {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === "true";
  } catch {
    return false;
  }
}

export default function Sidebar() {
  const { boardId } = useParams();
  const [members, setMembers] = useState(
    useLoaderData<typeof boardViewLoader>().members,
  );
  const [collapsed, setCollapsed] = useState(readCollapsed);

  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSED_KEY, String(collapsed));
    } catch {
      // Not remembered this time; the toggle still works.
    }
  }, [collapsed]);

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const email = (formData.get("email") as string).trim();
    if (!email) {
      return;
    }
    const res = await apiFetch(`/api/boards/${boardId}/invite`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        email,
      }),
    });
    if (!res.ok) {
      return;
    }

    setMembers((members) =>
      members.includes(email) ? members : [...members, email],
    );
  };

  const toggleLabel = collapsed ? "Expand member list" : "Collapse member list";

  return (
    <aside
      id="member-list"
      aria-label="Members"
      className={`z-10 h-full shrink-0 py-4 bg-sidebar border-r border-sidebar-border flex flex-col overflow-hidden transition-[width] duration-200 ${collapsed ? "w-14" : "w-xs"}`}
    >
      <div
        className={`flex items-center gap-2 pb-3 mb-2 border-b border-sidebar-border ${collapsed ? "justify-center px-2" : "px-6 pr-3"}`}
      >
        {!collapsed && (
          <>
            <SquareKanban className="size-5 shrink-0 text-primary" />
            <h2 className="flex-1 text-lg font-bold">
              Members
              <span className="ml-2 font-normal text-muted-foreground">
                ({members.length})
              </span>
            </h2>
          </>
        )}
        <Button
          variant="ghost"
          size="icon"
          className="size-8 shrink-0 text-muted-foreground"
          aria-label={toggleLabel}
          title={toggleLabel}
          aria-expanded={!collapsed}
          aria-controls="member-list"
          onClick={() => setCollapsed((collapsed) => !collapsed)}
        >
          {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
        </Button>
      </div>
      <div className={collapsed ? "px-2" : "px-4"}>
        <div className="space-y-1 mt-1">
          {members.map((member) => (
            <div
              key={member}
              title={collapsed ? member : undefined}
              className={`flex items-center gap-2 py-1.5 rounded-md hover:bg-sidebar-accent ${collapsed ? "justify-center" : "px-2"}`}
            >
              <UserAvatar
                email={member}
                className="ring-1 ring-border size-7"
              />
              {!collapsed && (
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-link truncate">{member}</p>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      <div
        className={`mt-auto pt-4 border-t border-sidebar-border ${collapsed ? "px-2" : "px-4"}`}
      >
        <Dialog>
          <DialogTrigger asChild>
            {collapsed ? (
              <Button
                variant="outline"
                size="icon"
                className="w-full"
                aria-label="Invite"
                title="Invite"
              >
                <Plus className="size-3" />
              </Button>
            ) : (
              <Button variant="outline" className="w-full">
                <span className="flex items-center">
                  <Plus className="size-3" />
                  <User className="size-4" />
                </span>
                Invite
              </Button>
            )}
          </DialogTrigger>
          <DialogContent>
            <form onSubmit={handleSubmit}>
              <DialogHeader className="mb-2">
                <DialogTitle>Invite people to your board</DialogTitle>
                <DialogDescription></DialogDescription>
              </DialogHeader>
              <Input
                type="email"
                required
                name="email"
                placeholder="Email address"
              />
              <DialogFooter className="mt-4">
                <DialogClose asChild>
                  <Button type="submit">Invite</Button>
                </DialogClose>
                <DialogClose asChild>
                  <Button variant="outline">Cancel</Button>
                </DialogClose>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>
    </aside>
  );
}
