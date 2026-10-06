import { Star } from "lucide-react";

import { Button } from "@/components/ui/button";

import { useFavorites } from "./favorites";
import type { Favorite } from "./loaders";

export default function FavoriteButton({ board }: { board: Favorite }) {
  const { favorites, toggleFavorite } = useFavorites();
  const favorite = favorites.some((f) => f.id === board.id);

  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={favorite ? "Remove from favorites" : "Add to favorites"}
      aria-pressed={favorite}
      onClick={(e) => {
        e.stopPropagation();
        toggleFavorite(board);
      }}
    >
      <Star
        className={
          favorite ? "fill-amber-400 text-amber-400" : "text-muted-foreground"
        }
      />
    </Button>
  );
}
