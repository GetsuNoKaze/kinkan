// The goals as the site shows them: the repository's file (.github/goals.json) with what the
// donations workflow counted added to each goal's sum. The pages workflow puts that ledger
// (donations.json on the "updates" release) into src/data before the build; without it the
// file's sums are shown as they are.
import file from '../../.github/goals.json';

type Ledger = { raised?: Record<string, Record<string, number>> };

const found = import.meta.glob<Ledger>('./data/donations.json', { eager: true, import: 'default' });
const ledger: Ledger = Object.values(found)[0] ?? {};

// Tribute counts in minimal units (cents); a goal shows whole ones, in its own currency only.
export const goals = {
	...file,
	items: file.items.map((g) => ({ ...g, raised: g.raised + Math.floor((ledger.raised?.[g.id]?.[g.currency] ?? 0) / 100) })),
};
