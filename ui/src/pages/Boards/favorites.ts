import { useOutletContext } from "react-router";

import type { Favorite, Me } from "./loaders";

export type FavoritesContext = {
  me: Me;
  favorites: Favorite[];
  toggleFavorite: (board: Favorite) => void;
};

export function useFavorites() {
  return useOutletContext<FavoritesContext>();
}

export function useMe() {
  return useOutletContext<FavoritesContext>().me;
}
