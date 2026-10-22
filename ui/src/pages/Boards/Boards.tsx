import { useState } from "react";
import { Outlet, useLoaderData } from "react-router";

import { make } from "@/offline/pending";
import { revisionOf } from "@/utils/revision";

import Header from "./Header";
import OfflineBanner from "./OfflineBanner";
import type { FavoritesContext } from "./favorites";
import type { boardsLoader, Favorite } from "./loaders";

// Keyed by the loader's data, so reloading it starts the layout afresh.
export default function BoardsRoute() {
  const loaderData = useLoaderData<typeof boardsLoader>();
  return <Boards key={revisionOf(loaderData)} loaderData={loaderData} />;
}

function Boards({
  loaderData,
}: {
  loaderData: Awaited<ReturnType<typeof boardsLoader>>;
}) {
  const [favorites, setFavorites] = useState(loaderData.favorites);
  const [me, setMe] = useState(loaderData.me);

  const toggleFavorite = async (board: Favorite) => {
    const old = favorites;
    const favorite = !old.some((f) => f.id === board.id);
    setFavorites(
      favorite ? [...old, board] : old.filter((f) => f.id !== board.id),
    );

    const outcome = await make({
      type: "favoriteBoard",
      boardId: board.id,
      favorite,
    });
    if (outcome === "dropped") setFavorites(old);
  };

  return (
    <div className="flex flex-col h-dvh bg-background">
      <Header me={me} setMe={setMe} favorites={favorites} />
      <OfflineBanner />
      {/* The page takes whatever height the header and banner leave. */}
      <div className="flex-1 min-h-0 overflow-y-auto">
        <Outlet
          context={{ me, favorites, toggleFavorite } satisfies FavoritesContext}
        />
      </div>
    </div>
  );
}
