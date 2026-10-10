import { Activity, Globe, ShieldCheck } from "lucide-react";
import { useState } from "react";
import type { Schemas } from "../../api/client";
import { Button } from "../../components/ui";
import { t } from "../../i18n";
import { ScannerDrawer } from "./node-scanners";
import { SiteDrawer } from "./node-site";
import { TTProbeDrawer } from "./node-ttprobe";

// Kinkan's buttons on a node's card, each with its drawer: the scanner journal, the check
// of the node's protocols and the node's site. nodes.tsx (Mikan's) places them with one
// line, so that a merge from upstream meets as few of the fork's lines as possible.

type Node = Schemas["NodeInfo"];
type Open = "scanners" | "probe" | "site" | null;

export function KinkanNodeButtons({ node }: { node: Node }) {
  const [open, setOpen] = useState<Open>(null);
  const close = () => setOpen(null);
  return <>
    <Button size="sm" onClick={() => setOpen("scanners")}><Activity size={16} aria-hidden />{t("scanners.title")}</Button>
    <Button size="sm" onClick={() => setOpen("probe")}>
      <ShieldCheck size={16} aria-hidden /> {t("ttProbe.button")}
    </Button>
    <Button size="sm" onClick={() => setOpen("site")}>
      <Globe size={16} aria-hidden /> {t("site.button")}
    </Button>
    <ScannerDrawer node={open === "scanners" ? node : null} onClose={close} />
    <TTProbeDrawer node={open === "probe" ? node : null} onClose={close} onOpenSite={() => setOpen("site")} />
    <SiteDrawer node={open === "site" ? node : null} onClose={close} />
  </>;
}
