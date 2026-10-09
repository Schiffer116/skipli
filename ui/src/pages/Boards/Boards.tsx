import { useState } from "react";
import { Outlet, useLoaderData } from "react-router";

import Header from "./Header";
import type { FavoritesContext } from "./favorites";
import type { boardsLoader, Favorite } from "./loaders";
import { apiFetch } from "@/utils/apiFetch";

export default function Boards() {
  const loaderData = useLoaderData<typeof boardsLoader>();
  const [favorites, setFavorites] = useState(loaderData.favorites);
  const [me, setMe] = useState(loaderData.me);

  const toggleFavorite = async (board: Favorite) => {
    const old = favorites;
    const favorite = !old.some((f) => f.id === board.id);
    setFavorites(
      favorite ? [...old, board] : old.filter((f) => f.id !== board.id),
    );

    const res = await apiFetch(`/api/boards/${board.id}/favorite`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ favorite }),
    });

    if (!res.ok) {
      setFavorites(old);
    }
  };

  return (
    <div className="flex flex-col min-h-screen bg-background">
      <Header me={me} setMe={setMe} favorites={favorites} />
      <Outlet
        context={{ favorites, toggleFavorite } satisfies FavoritesContext}
      />
    </div>
  );
}
