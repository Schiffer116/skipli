import { useState } from "react";
import { Users } from "lucide-react";

import UserAvatar from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { updateBoard } from "@/offline/copy";
import { useSync } from "@/offline/sync";
import { apiFetch } from "@/utils/apiFetch";

import { useMe } from "../favorites";
import type { Member } from "../loaders";

// Who may do what, in one place for roles to slot into later: today every
// member can invite, and only the owner can edit or delete the board.
const canInvite = () => true;

export default function ShareDialog({
  boardId,
  name,
  owner,
  members: loaded,
}: {
  boardId: string;
  name: string;
  owner: string;
  members: Member[];
}) {
  const me = useMe();
  // Inviting needs the server, to send the invitation email.
  const { offline } = useSync();
  const [members, setMembers] = useState(loaded);
  // The server's members, once they come, replace the ones loaded first.
  const [shown, setShown] = useState(loaded);
  if (loaded !== shown) {
    setShown(loaded);
    setMembers(loaded);
  }
  const [email, setEmail] = useState("");
  const [failed, setFailed] = useState(false);
  const [inviting, setInviting] = useState(false);

  const invite = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const address = email.trim();
    if (!address || offline || inviting) return;
    setInviting(true);
    setFailed(false);
    try {
      const res = await apiFetch(`/api/boards/${boardId}/invite`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: address }),
      });
      if (!res.ok) {
        setFailed(true);
        return;
      }
    } catch {
      setFailed(true);
      return;
    } finally {
      setInviting(false);
    }

    const invited = members.some((member) => member.email === address)
      ? members
      : [
          ...members,
          { id: address, email: address, name: address.split("@")[0] },
        ];
    setMembers(invited);
    setEmail("");
    // The board is Shared now, so it must be read-only when offline.
    void updateBoard(me.id, boardId, { members: invited });
  };

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button aria-label="Share" title="Share">
          <Users />
          <span className="max-md:hidden">Share</span>
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Share "{name}"</DialogTitle>
          <DialogDescription className="sr-only">
            Invite people to this board and see who has access.
          </DialogDescription>
        </DialogHeader>
        {canInvite() && (
          <form onSubmit={invite} className="flex flex-col gap-2">
            <div className="flex gap-2">
              <Input
                type="email"
                required
                name="email"
                aria-label="Email address"
                placeholder="Add people by email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <Button type="submit" disabled={offline || inviting}>
                Invite
              </Button>
            </div>
            {offline && (
              <p className="text-xs text-muted-foreground">
                Inviting needs a connection
              </p>
            )}
            {failed && (
              <p role="alert" className="text-xs text-destructive">
                Couldn't invite that address. Check it and try again.
              </p>
            )}
          </form>
        )}
        <section aria-label="People with access">
          <h3 className="mb-2 text-sm font-semibold">People with access</h3>
          <ul className="flex max-h-64 flex-col gap-1 overflow-y-auto">
            {members.map((member) => (
              <li key={member.id} className="flex items-center gap-3 py-1">
                <UserAvatar
                  email={member.email}
                  name={member.name}
                  className="size-8 shrink-0 ring-1 ring-border"
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm">
                    {member.name}
                    {member.id === me.id && (
                      <span className="text-muted-foreground"> (you)</span>
                    )}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {member.email}
                  </p>
                </div>
                {member.id === owner && (
                  <span className="shrink-0 text-xs text-muted-foreground">
                    Owner
                  </span>
                )}
              </li>
            ))}
          </ul>
        </section>
      </DialogContent>
    </Dialog>
  );
}
