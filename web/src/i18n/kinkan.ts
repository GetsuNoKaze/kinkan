import type kinkanRu from "./kinkan.ru.json";

// Kinkan's texts live in kinkan.ru.json / kinkan.en.json, apart from Mikan's dictionaries,
// so that a merge from upstream never meets them. They are laid over Mikan's when a
// language is loaded. The fork adds keys and changes Mikan's only to call the product
// Kinkan: Mikan's NOTICE.md asks a modified version not to present itself as mikan.

export type KinkanDict = typeof kinkanRu;

type Module<D> = Promise<{ default: D }>;

/** Loads Mikan's dictionary and the fork's together, the fork's keys laid over. */
export function withKinkan<D extends object>(mikan: () => Module<D>, kinkan: () => Module<KinkanDict>): () => Module<D & KinkanDict> {
  return async () => {
    const [m, k] = await Promise.all([mikan(), kinkan()]);
    return { default: merge(m.default, k.default) as D & KinkanDict };
  };
}

function merge(base: object, over: object): object {
  const out: Record<string, unknown> = { ...(base as Record<string, unknown>) };
  for (const [key, value] of Object.entries(over)) {
    const prev = out[key];
    out[key] = isObject(prev) && isObject(value) ? merge(prev, value) : value;
  }
  return out;
}

function isObject(v: unknown): v is object {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}
