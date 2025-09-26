import { Link, useLoaderData } from "react-router";
import type { boardsLoader } from "./Boards";
import { Avatar, AvatarImage } from "@/components/ui/avatar";

import md5 from "md5";

export default function Header() {
  const { email } = useLoaderData<typeof boardsLoader>();

  const hash = md5(email);
  const avatarUrl = `https://www.gravatar.com/avatar/${hash}?d=identicon`;

  return (
    <header>
      <div className="sticky top-0 left-0 bg-white p-4 w-screen border-b border-orange-200 flex items-center gap-2 justify-between">
        <Link to="/boards" className="flex items-center">
          <img src="/skipli.png" alt="Logo" width={40} height={40} />
          <p className="flex-grow-0 font-bold text-lg text-primary">Skipli</p>
        </Link>

        <div className="flex items-center gap-2">
          <p>{email}</p>
          <Avatar className="border-2 border-primary">
            <AvatarImage src={avatarUrl} alt="@shadcn" />
          </Avatar>
        </div>
      </div>
    </header>
  );
}
