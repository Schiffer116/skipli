import { useOutletContext } from "react-router";

import type { Favorite } from "./loaders";

export type FavoritesContext = {
  favorites: Favorite[];
  toggleFavorite: (board: Favorite) => void;
};

export function useFavorites() {
  return useOutletContext<FavoritesContext>();
}
