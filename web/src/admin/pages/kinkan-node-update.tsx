import { useQuery } from "@tanstack/react-query";
import { Copy } from "lucide-react";
import { api, unwrap, type Schemas } from "../../api/client";
import { t } from "../../i18n";
import { useCopy } from "../../lib/copy";

// Kinkan: a node's failed update, where Mikan's text would mislead. nodes.tsx (Mikan's)
// hands such a failure here with one line.
//
// A node installed with Mikan's installer knows only Mikan's signed releases and refuses
// the fork's, so the panel cannot move it to Kinkan: that takes the fork's installer, run
// once on the node's server. After that the node follows the panel like any other. And the
// updater reports the version it runs as the one it failed to reach when it found nothing to
// go to, which Mikan's text shows as "could not update to <the same version>".

type Node = Schemas["NodeInfo"];

const notOurs = "is not a release of this server's channel";

export function kinkanNodeUpdate(n: Node): boolean {
  const u = n.update;
  if (u?.state !== "failed") return false;
  return (u.error ?? "").includes(notOurs) || u.version === (u.from || n.version);
}

export function KinkanNodeUpdate({ n }: { n: Node }) {
  const copyText = useCopy();
  const panel = useQuery({ queryKey: ["kinkan-rollback"], queryFn: () => unwrap(api.GET("/api/v1/updates/rollback")) });
  const u = n.update;
  const from = u?.from || n.version || "";
  const error = (u?.error ?? "").split("\n")[0] ?? "";
  const version = panel.data?.current;
  if (!error.includes(notOurs) || !version?.includes("-tt.")) {
    return <p className="mt-3 text-[13px] text-[var(--berry-600)]" role="alert">{t("kinkanNodeUpdate.failed", { from, e: error })}</p>;
  }
  const command = `curl -fsSL https://github.com/GetsuNoKaze/kinkan/releases/download/v${version}/install.sh | sudo bash`;
  return (
    <div className="mt-3">
      <p className="mb-2 text-[13px] text-[var(--ink-500)]">{t("kinkanNodeUpdate.onMikan", { from, v: version })}</p>
      <div className="link-field">
        <span className="mono break-all text-xs">{command}</span>
        <button type="button" className="icon-btn" onClick={() => void copyText(command, t("nodes.commandCopied"))} aria-label={t("nodes.copyCommand")}>
          <Copy size={18} />
        </button>
      </div>
    </div>
  );
}
