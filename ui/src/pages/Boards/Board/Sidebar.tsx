import { Avatar, AvatarImage } from "@/components/ui/avatar";
import { Plus, SquareKanban, User } from "lucide-react";
import { useLoaderData, useParams } from "react-router";
import md5 from "md5";
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
import type { boardViewLoader } from "../BoardView";

export default function Sidebar() {
  const { boardId } = useParams();
  const { members } = useLoaderData<typeof boardViewLoader>();

  const avatarUrls = members.map((member) => {
    const hash = md5(member);
    return `https://www.gravatar.com/avatar/${hash}?d=identicon`;
  });

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const email = formData.get("email");
    if (!email) {
      return;
    }
    await fetch(`/api/boards/${boardId}/invite`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        email,
      }),
    });
  };

  return (
    <div className="sticky z-10 top-12 left-0 h-[calc(100vh-3rem)] w-xs py-4 bg-sidebar border-r border-sidebar-border flex flex-col min-w-xs">
      <div className="flex items-center gap-2 px-6 pb-3 mb-2 border-b border-sidebar-border">
        <SquareKanban className="size-5 text-primary" />
        <h2 className="text-lg font-bold">Members</h2>
      </div>
      <div className="px-4 space-y-1">
        <div className="space-y-1 mt-1">
          {members.map((member, idx) => (
            <div
              key={member}
              className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-sidebar-accent"
            >
              <Avatar className="ring-1 ring-border size-7">
                <AvatarImage src={avatarUrls[idx]} alt={member} />
              </Avatar>
              <div className="flex-1 min-w-0">
                <p className="text-sm text-link truncate">{member}</p>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="mt-auto px-4 pt-4 border-t border-sidebar-border">
        <Dialog>
          <DialogTrigger asChild>
            <Button
              variant="outline"
              className="flex items-center justify-center gap-2 w-full cursor-pointer"
              asChild
            >
              <div>
                <div className="flex items-center">
                  <Plus className="size-3" />
                  <User className="size-4" />
                </div>
                <span>Invite</span>
              </div>
            </Button>
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
    </div>
  );
}
