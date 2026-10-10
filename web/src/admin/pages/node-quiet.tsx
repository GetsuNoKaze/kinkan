import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Globe, PowerOff, RotateCcw } from "lucide-react";
import { useState } from "react";
import { api, errorText, unwrap, type Schemas } from "../../api/client";
import { Confirm } from "../../components/overlay";
import { useToast } from "../../components/toast";
import { Button, Pill } from "../../components/ui";
import { t, tMaybe } from "../../i18n";

// Kinkan: the quiet node. What the node as a whole looks like to a stranger (as quiet as
// its loudest inbound) and, for each inbound that gives it away, why and what helps.

type Quiet = Schemas["QuietView"];
type Advice = Schemas["QuietAdvice"];

const tone = (level: string) => (level === "exposed" ? "bad" : level === "noticeable" ? "warn" : "off");
const banner = (verdict: string) => (verdict === "quiet" ? "info" : verdict === "exposed" ? "err" : "warn");

export function QuietSummary({ quiet, onSite, onReference }: { quiet: Quiet; onSite?: () => void; onReference: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [disabling, setDisabling] = useState<Advice | null>(null);
  const [disabled, setDisabled] = useState<number[]>([]);
  const disable = useMutation({
    mutationFn: (a: Advice) => unwrap(api.PATCH("/api/v1/inbounds/{id}", { params: { path: { id: a.inbound_id } }, body: { enabled: false } })),
    onSuccess: async (_, a) => {
      setDisabling(null);
      setDisabled((list) => [...list, a.inbound_id]);
      toast.ok(t("quietNode.disabledDone", { name: a.name }));
      await qc.invalidateQueries({ queryKey: ["inbounds"] });
    },
    onError: (e) => toast.error(errorText(e)),
  });
  return <section className="space-y-3" aria-labelledby="quiet-node-title">
    <h3 id="quiet-node-title" className="text-sm font-medium">{t("quietNode.title")}</h3>
    <div role="status" className={`banner ${banner(quiet.verdict)}`}>{tMaybe(`quietNode.verdict.${quiet.verdict}`) ?? quiet.verdict}</div>
    {quiet.advice.length > 0 ? <ul className="space-y-2">
      {quiet.advice.map((a) => <li key={`${a.inbound_id}/${a.code}`} className="space-y-2 rounded-xl border border-[var(--hairline)] p-3 text-sm">
        <div className="flex flex-wrap items-center gap-2"><Pill tone={tone(a.level)}>{tMaybe(`quietNode.level.${a.level}`) ?? a.level}</Pill><span className="font-medium break-words">{a.name}</span></div>
        <p>{tMaybe(`quietNode.advice.${a.code}`, a.params ?? undefined) ?? a.code}</p>
        {a.action === "disable" ? (disabled.includes(a.inbound_id)
          ? <p className="text-xs text-[var(--ink-500)]">{t("quietNode.disabledDone", { name: a.name })}</p>
          : <Button size="sm" onClick={() => setDisabling(a)}><PowerOff size={16} aria-hidden />{t("quietNode.disable")}</Button>) : null}
        {a.action === "site" && onSite ? <Button size="sm" onClick={onSite}><Globe size={16} aria-hidden />{t("quietNode.site")}</Button> : null}
        {a.action === "reference" ? <Button size="sm" onClick={onReference}><RotateCcw size={16} aria-hidden />{t("quietNode.reference")}</Button> : null}
      </li>)}
    </ul> : null}
    <p className="text-xs text-[var(--ink-500)]">{t("quietNode.why")}</p>
    <Confirm
      open={!!disabling}
      onOpenChange={(v) => !v && setDisabling(null)}
      title={t("quietNode.disableTitle", { name: disabling?.name ?? "" })}
      text={t("quietNode.disableText")}
      confirm={t("quietNode.disable")}
      danger
      loading={disable.isPending}
      onConfirm={() => disabling && disable.mutate(disabling)}
    />
  </section>;
}
