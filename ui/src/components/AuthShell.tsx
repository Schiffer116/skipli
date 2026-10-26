import {
  BookOpen,
  Briefcase,
  CircleCheck,
  House,
  Lightbulb,
  MapPin,
  Smartphone,
  Sparkles,
  WifiOff,
  Zap,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode } from "react";

const useCases: [LucideIcon, string][] = [
  [Briefcase, "Team projects"],
  [House, "Household chores"],
  [MapPin, "Trip planning"],
  [BookOpen, "Study plans"],
  [Lightbulb, "Side projects"],
];

const features: [LucideIcon, string, string][] = [
  [
    Zap,
    "Real-time collaboration",
    "Invite your team by email. Every change shows up on everyone's screen instantly.",
  ],
  [
    WifiOff,
    "Offline",
    "Keep working without a connection. Changes sync when you're back.",
  ],
  [
    Sparkles,
    "Animations",
    "Smooth drag and drop and transitions that make every move feel natural.",
  ],
  [Smartphone, "Mobile", "Install it like an app and swipe between columns."],
];

const columns: { title: string; tasks: [string, boolean][] }[] = [
  {
    title: "Launch",
    tasks: [
      ["Draft launch email", true],
      ["Pick a demo date", false],
      ["Update the roadmap", false],
      ["Plan team offsite", true],
      ["Book meeting room", false],
    ],
  },
  {
    title: "Website",
    tasks: [
      ["Fix mobile swipe", true],
      ["Design sign-in", false],
    ],
  },
  {
    title: "Team",
    tasks: [
      ["Renew the domain", false],
      ["Move standup to 9:30 on Mondays", true],
      ["Ask Linh about budget", true],
      ["Order team T-shirts", false],
    ],
  },
];

const task =
  "flex items-center gap-1.5 rounded-md border px-2.5 py-2 text-[13px]";

function BoardIllustration() {
  return (
    <div
      aria-hidden
      className="overflow-hidden rounded-xl border bg-card shadow-xl"
    >
      <div className="flex h-11 items-center justify-between gap-3 bg-nav px-4">
        <div className="flex items-center gap-2.5">
          <span className="text-sm font-bold text-nav-foreground">Team A</span>
          <span className="text-xs text-nav-muted">· Launch week</span>
        </div>
        <span className="flex">
          <span className="flex size-6.5 items-center justify-center rounded-full border-2 border-nav bg-primary text-[11px] font-bold text-primary-foreground">
            L
          </span>
          <span className="-ml-2 flex size-6.5 items-center justify-center rounded-full border-2 border-nav bg-link text-[11px] font-bold text-white">
            M
          </span>
        </span>
      </div>
      <div className="relative grid grid-cols-3 items-start gap-3 bg-muted p-4">
        {columns.map(({ title, tasks }) => (
          <div
            key={title}
            className="flex flex-col gap-2 rounded-lg border bg-card p-3"
          >
            <span className="text-[13px] font-bold">{title}</span>
            {tasks.map(([name, done]) => (
              <span
                key={name}
                className={`${task} ${name === "Design sign-in" ? "opacity-50" : ""}`}
              >
                {done && <CircleCheck className="size-3.5 text-success" />}
                {name}
              </span>
            ))}
            {title !== "Team" && (
              <span className="px-0.5 py-1 text-xs text-muted-foreground">
                + Create a task
              </span>
            )}
          </div>
        ))}
        <div className="absolute top-21 left-[calc((100%-56px)*0.4833+48px)] box-border w-[calc((100%-56px)/3-26px)] animate-[sk-float_2.4s_ease-in-out_infinite] cursor-grabbing rounded-md border bg-card px-2.5 py-2 text-[13px] shadow-lg motion-reduce:animate-none">
          Design sign-in
        </div>
      </div>
    </div>
  );
}

/** Marketing panel on the left, dark sign-in panel on the right. */
export default function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="grid min-h-screen grid-cols-1 bg-background text-foreground min-[861px]:grid-cols-[minmax(0,1.618fr)_minmax(0,1fr)]">
      <main className="flex min-w-0 flex-col gap-8 px-5 pt-8 pb-14 min-[861px]:px-8">
        <div className="flex items-center gap-3">
          <img src="/skipli.png" alt="" className="size-10" />
          <span className="text-2xl font-extrabold tracking-tight text-heading">
            Skipli
          </span>
        </div>
        <section
          aria-labelledby="tagline"
          className="mx-auto flex w-full max-w-[820px] grow flex-col justify-center gap-8"
        >
          <div className="flex flex-col gap-3">
            <h1
              id="tagline"
              className="text-4xl leading-tight font-bold tracking-tight text-heading min-[861px]:text-[42px]"
            >
              Organize anything, together.
            </h1>
            <p className="text-lg leading-relaxed text-muted-foreground">
              Skipli gives you and your team simple boards, cards and tasks —
              for planning projects, sharing chores, or anything in between.
            </p>
            <ul
              aria-label="What people use Skipli for"
              className="mt-2 flex flex-wrap gap-2"
            >
              {useCases.map(([Icon, label]) => (
                <li
                  key={label}
                  className="flex items-center gap-1.5 rounded-full border bg-card px-3 py-1.5 text-sm"
                >
                  <Icon className="size-4 text-muted-foreground" aria-hidden />
                  {label}
                </li>
              ))}
            </ul>
          </div>
          <BoardIllustration />
          <ul className="grid grid-cols-1 gap-x-8 gap-y-5 sm:grid-cols-2">
            {features.map(([Icon, title, text]) => (
              <li key={title} className="flex items-start gap-3">
                <Icon
                  className="mt-px size-5.5 shrink-0 text-primary"
                  aria-hidden
                />
                <div className="flex flex-col gap-0.5">
                  <span className="font-bold">{title}</span>
                  <span className="text-sm leading-snug text-muted-foreground">
                    {text}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        </section>
      </main>

      {/* Always dark, whatever the theme: `dark` swaps in the dark tokens. */}
      <section
        aria-label="Sign in"
        className="dark flex min-w-0 flex-col gap-14 border-t-4 border-primary min-[861px]:border-t-0 min-[861px]:border-l-4 bg-nav px-5 pt-20 pb-8 text-nav-foreground min-[861px]:px-14"
      >
        <div className="mx-auto my-auto flex w-full max-w-[400px] flex-col gap-8">
          <img
            src="/skipli.png"
            alt=""
            className="size-10 rounded-[10px] bg-white p-2"
          />
          {children}
        </div>
        <footer className="mx-auto flex flex-wrap justify-center gap-x-5 gap-y-2 text-[13px] text-nav-muted">
          <span>© 2026 Skipli</span>
          <a
            href="https://github.com/Schiffer116/skipli"
            className="text-link hover:underline"
          >
            GitHub
          </a>
        </footer>
      </section>
    </div>
  );
}
