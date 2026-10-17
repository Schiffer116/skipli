const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ["day", 24 * 60 * 60 * 1000],
  ["hour", 60 * 60 * 1000],
  ["minute", 60 * 1000],
];

const format = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

// "5 minutes ago", "yesterday": how long ago something was saved.
export function ago(time: number, now = Date.now()) {
  const elapsed = now - time;
  for (const [unit, size] of units) {
    if (elapsed >= size)
      return format.format(-Math.floor(elapsed / size), unit);
  }
  return "just now";
}
