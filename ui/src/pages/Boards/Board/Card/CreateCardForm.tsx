import { useContext, useRef } from "react";
import { useParams } from "react-router";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

import { BoardContext } from "../BoardContext";
import useEscape from "@/hooks/useEscape";
import useOutsidePointerDown from "@/hooks/useOutsidePointerDown";
import { newId } from "@/offline/ids";

export default function CreateCard() {
  const boardId = useParams().boardId!;

  const { make, setShowCreateCardForm } = useContext(BoardContext)!;
  useEscape(() => setShowCreateCardForm(false));
  const formRef = useRef<HTMLFormElement>(null);
  useOutsidePointerDown(formRef, () => setShowCreateCardForm(false));

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name") as string;
    if (!name) {
      return;
    }

    void make({
      type: "createCard",
      boardId,
      cardId: newId(),
      name,
      createdAt: new Date().toISOString(),
    });
    setShowCreateCardForm(false);
  };

  return (
    <form ref={formRef} onSubmit={handleSubmit}>
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
