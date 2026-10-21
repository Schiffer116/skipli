import { useContext } from "react";
import { useParams } from "react-router";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

import { socket } from "@/utils/socket";
import { BoardContext } from "../BoardContext";
import { createCard } from "@/utils/updateCard";
import useEscape from "@/hooks/useEscape";
import { newId } from "@/offline/ids";
import { submit } from "@/offline/sync";

export default function CreateCard() {
  const boardId = useParams().boardId!;

  const { setCards, setShowCreateCardForm } = useContext(BoardContext)!;
  useEscape(() => setShowCreateCardForm(false));

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name") as string;
    if (!name) {
      return;
    }

    const cardId = newId();
    const saved = await submit({
      type: "createCard",
      boardId,
      cardId,
      name,
      createdAt: new Date().toISOString(),
    });
    if (!saved) {
      return;
    }

    const newCard = { id: cardId, name, description: "", tasks: [] };

    socket.emit("create card", newCard);
    setCards(createCard(newCard));
    setShowCreateCardForm(false);
  };

  return (
    <form onSubmit={handleSubmit}>
      <Card className="w-full bg-card border-border">
        <CardHeader>
          <CardTitle className="text-heading">Create a card</CardTitle>
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
              type="button"
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
