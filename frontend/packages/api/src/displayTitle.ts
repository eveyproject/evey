// What a card or list row should call an item. The server stamps seriesTitle
// on episode and season payloads, so naming the show costs no extra request;
// items without the stamp keep their own title, never a broken prefix.
type StampedItem = { title?: string; seriesTitle?: string };

export function displayTitle(item: StampedItem): string {
  if (item.seriesTitle && item.title) return `${item.seriesTitle} - ${item.title}`;
  return item.seriesTitle || item.title || "";
}
