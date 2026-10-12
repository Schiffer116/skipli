import { Scale } from "lucide-react";

import { Button } from "@/components/ui/button";
import { apiFetch } from "@/utils/apiFetch";
import { rebalance } from "@/utils/rebalance";
import { socket } from "@/utils/socket";
import { reorderCards } from "@/utils/updateCard";

import type { CardType } from "./Card/Card";

type RebalanceButtonProps = {
  boardId: string;
  cards: CardType[];
  setCards: React.Dispatch<React.SetStateAction<CardType[]>>;
  columns: number;
};

export default function RebalanceButton({
  boardId,
  cards,
  setCards,
  columns,
}: RebalanceButtonProps) {
  const onClick = async () => {
    const cardIds = rebalance(cards, columns).map((card) => card.id);
    if (cardIds.every((id, i) => id === cards[i].id)) return;

    const oldCards = cards;
    setCards(reorderCards(cardIds));

    const res = await apiFetch(`/api/boards/${boardId}/cards/order`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ cardIds }),
    });
    if (!res.ok) {
      setCards(oldCards);
      return;
    }

    socket.emit("reorder cards", cardIds);
  };

  return (
    <Button variant="outline" onClick={onClick}>
      <Scale />
      Rebalance
    </Button>
  );
}
