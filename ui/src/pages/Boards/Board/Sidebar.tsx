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
    <div className="sticky z-10 top-0 left-0 w-xs p-4 bg-white border-r border-orange-200 shadow-sm flex flex-col min-w-xs">
      <div className="flex items-center text-lg gap-2 p-2 group">
        <SquareKanban className="h-10 w-10 text-orange-400" />
        <h2>Members</h2>
      </div>
      <div className="px-2 space-y-1">
        <div className="space-y-1 mt-1">
          {members.map((member, idx) => (
            <div
              key={member}
              className="flex items-center gap-2 p-2 rounded-md"
            >
              <Avatar className="border-2 border-gray-500 h-8 w-8">
                <AvatarImage src={avatarUrls[idx]} alt={member} />
              </Avatar>
              <div className="flex-1 min-w-0">
                <p className="font-medium text-gray-700 truncate">{member}</p>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="absolute bottom-0 left-0 w-full">
        <Dialog>
          <DialogTrigger asChild>
            <Button
              variant="ghost"
              className="flex h-14 items-center justify-center gap-2 w-full text-lg rounded-none border-t-3 border-t-gray-200"
              asChild
            >
              <div>
                <div className="flex items-center">
                  <Plus className="size-4" />
                  <User className="size-6" />
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
