import { useContext } from "react";
import { useParams } from "react-router";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

import { socket } from "@/main";
import { BoardContext } from "../Board";
import { createCard } from "@/utils/updateCard";

export default function CreateCard() {
  const boardId = useParams().boardId!;

  const { setCards, setShowCreateCardForm } = useContext(BoardContext)!;

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name");
    if (!name) {
      return;
    }

    const data = await fetch(`/api/boards/${boardId}/cards`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
      },
      body: JSON.stringify({
        name,
        description: "",
        createdAt: new Date().toISOString(),
      }),
    });

    if (!data.ok) {
      return;
    }

    const newCard = {
      ...(await data.json()),
      tasks: [],
    };

    socket.emit("create card", newCard);
    setCards(createCard(newCard));
    setShowCreateCardForm(false);
  };

  return (
    <form onSubmit={handleSubmit}>
      <Card className="flex-shrink-0 w-80 bg-card border-border">
        <CardHeader>
          <CardTitle>Create a card</CardTitle>
        </CardHeader>
        <CardContent>
          <Input name="name" placeholder="Name" className="mb-3" autoFocus />
          <div className="space-x-2">
            <Button
              type="submit"
              size="sm"
              className="bg-primary hover:bg-primary/90"
            >
              Create
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowCreateCardForm(false)}
            >
              Cancel
            </Button>
          </div>
        </CardContent>
      </Card>
    </form>
  );
}
