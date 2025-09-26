import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

import type { Board } from "./BoardList";

type BoardFormProps = {
  setBoards: React.Dispatch<React.SetStateAction<Board[]>>;
  setShowCreateBoardForm: React.Dispatch<React.SetStateAction<boolean>>;
};

export default function CreateBoardForm(props: BoardFormProps) {
  const { setBoards, setShowCreateBoardForm } = props;

  const createBoard = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name") as string;
    const description = formData.get("description") as string;
    if (!name) {
      return;
    }

    const data = await fetch("/api/boards", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
      },
      body: JSON.stringify({
        name,
        description,
      }),
    });

    if (!data.ok) {
      return;
    }

    const newBoard: Board = await data.json();
    setBoards((boards) => [...boards, newBoard]);
    setShowCreateBoardForm(false);
  };

  return (
    <div className="w-full max-w-xs aspect-video">
      <Card className="aspect-video w-full">
        <CardHeader>
          <CardTitle>Create a new board</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={createBoard} className="space-y-3">
            <Input
              id="edit-name"
              name="name"
              type="text"
              placeholder="Name"
              autoFocus
            />
            <Input
              id="edit-description"
              name="description"
              type="text"
              placeholder="Description"
            />
            <div className="space-x-2">
              <Button type="submit">Create</Button>
              <Button
                variant="outline"
                onClick={() => setShowCreateBoardForm(false)}
              >
                Cancel
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
