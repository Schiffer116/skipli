import type { CardType } from "@/pages/Boards/Board/Card";

// Rough heights in task rows: a card's header and footer take about three,
// and the "Create a card" button after the last card about one.
const CARD_BASE = 3;
const CREATE_CARD = 1;

const height = (card: CardType) => card.tasks.length + CARD_BASE;

// Masonry places card i in column i % columns, so how many cards each
// column holds is fixed. This picks which cards share a column so the
// column heights vary as little as possible, then lays them back out row by
// row, keeping each column's cards in their old relative order.
export function rebalance(cards: CardType[], columns: number) {
  const capacity = Array.from(
    { length: columns },
    (_, c) =>
      Math.floor(cards.length / columns) + (c < cards.length % columns ? 1 : 0),
  );
  const groups: CardType[][] = capacity.map(() => []);
  const totals = capacity.map((_, c) =>
    c === cards.length % columns ? CREATE_CARD : 0,
  );

  for (const card of [...cards].sort((a, b) => height(b) - height(a))) {
    let lightest = -1;
    groups.forEach((group, c) => {
      if (
        group.length < capacity[c] &&
        (lightest === -1 || totals[c] < totals[lightest])
      ) {
        lightest = c;
      }
    });
    groups[lightest].push(card);
    totals[lightest] += height(card);
  }

  // Swapping cards of heights x > y from a taller column into a shorter one
  // lowers the variance exactly when x - y is less than the columns' gap.
  let improved = true;
  while (improved) {
    improved = false;
    for (let a = 0; a < columns; a++) {
      for (let b = 0; b < columns; b++) {
        for (let i = 0; i < groups[a].length; i++) {
          for (let j = 0; j < groups[b].length; j++) {
            const diff = height(groups[a][i]) - height(groups[b][j]);
            if (diff > 0 && diff < totals[a] - totals[b]) {
              [groups[a][i], groups[b][j]] = [groups[b][j], groups[a][i]];
              totals[a] -= diff;
              totals[b] += diff;
              improved = true;
            }
          }
        }
      }
    }
  }

  const position = new Map(cards.map((card, i) => [card.id, i]));
  groups.forEach((group) =>
    group.sort((x, y) => position.get(x.id)! - position.get(y.id)!),
  );

  return cards.map((_, i) => groups[i % columns][Math.floor(i / columns)]);
}
