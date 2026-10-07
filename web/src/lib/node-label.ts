import { t } from "../i18n";

/** A node's name for the admin: the panel's own node may have none yet. */
export function nodeLabel(n: { name: string; local: boolean }): string {
  return n.name || (n.local ? t("nodes.localName") : "—");
}
