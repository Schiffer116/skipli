import type { CardType } from "@/pages/Boards/Board/Card";

// A rough card height in task rows: the header and footer take about three.
const CARD_BASE = 3;

export const estimateHeight = (card: CardType) => card.tasks.length + CARD_BASE;

// Takes the cards in order and puts each at the bottom of whichever column
// is shortest so far (the leftmost on a tie), then the "Create a card"
// button the same way, as if it were the next card. Heights are estimated
// by task count, so the columns come straight from the card order without
// measuring anything first. Returns each column's card ids, top to bottom,
// and the column the button goes in.
export function placeColumns(
  ids: string[],
  heights: Record<string, number>,
  count: number,
) {
  const columns: string[][] = Array.from({ length: count }, () => []);
  const totals = columns.map(() => 0);
  const shortest = () => totals.indexOf(Math.min(...totals));

  for (const id of ids) {
    const column = shortest();
    columns[column].push(id);
    totals[column] += heights[id];
  }
  return { columns, createColumn: shortest() };
}
