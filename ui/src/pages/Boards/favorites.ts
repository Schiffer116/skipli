import { useOutletContext } from "react-router";

import { GUEST } from "@/offline/guest";

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

// Whether this is a guest, using Skipli without an account.
export const useGuest = () => useMe().id === GUEST.id;
