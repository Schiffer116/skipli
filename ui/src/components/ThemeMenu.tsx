import { useState } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Check, Monitor, Moon, Sun } from "lucide-react";

import { readTheme, setTheme, type Theme } from "@/utils/theme";

const themes = [
  { value: "system", label: "System", Icon: Monitor },
  { value: "light", label: "Light", Icon: Sun },
  { value: "dark", label: "Dark", Icon: Moon },
] satisfies { value: Theme; label: string; Icon: typeof Sun }[];

// The header's light, dark or system choice. Applies at once: it's
// remembered on this device, not on the account, so guests have it too.
export default function ThemeMenu() {
  const [theme, setChoice] = useState(readTheme);
  const { Icon, label } = themes.find(({ value }) => value === theme)!;

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={`Appearance: ${label}`}
          title="Appearance"
          className="flex size-8 items-center justify-center rounded text-nav-muted outline-none hover:bg-nav-muted/15 hover:text-nav-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50"
        >
          <Icon className="size-4" aria-hidden />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={6}
          className="z-50 min-w-40 rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
        >
          <DropdownMenu.Label className="px-2 py-1.5 text-xs text-muted-foreground">
            Appearance
          </DropdownMenu.Label>
          <DropdownMenu.RadioGroup
            value={theme}
            onValueChange={(value) => {
              setTheme(value as Theme);
              setChoice(value as Theme);
            }}
          >
            {themes.map(({ value, label, Icon }) => (
              <DropdownMenu.RadioItem
                key={value}
                value={value}
                className="flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none select-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground"
              >
                <Icon className="size-4 text-muted-foreground" aria-hidden />
                {label}
                <DropdownMenu.ItemIndicator className="ml-auto">
                  <Check className="size-4" aria-hidden />
                </DropdownMenu.ItemIndicator>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
          <p className="max-w-48 px-2 py-1.5 text-xs text-muted-foreground">
            Remembered on this device. System follows your device's setting.
          </p>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
