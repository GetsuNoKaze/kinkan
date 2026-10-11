import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorText, unwrap } from "../../../api/client";
import { qk } from "../../../api/hooks";
import { Confirm } from "../../../components/overlay";
import { Button } from "../../../components/ui";
import { useToast } from "../../../components/toast";
import { t } from "../../../i18n";

export function KinkanRollback({ busy }: { busy: boolean }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [confirm, setConfirm] = useState(false);
  const [requested, setRequested] = useState(false);
  const rollback = useQuery({ queryKey: ["kinkan-rollback"], queryFn: () => unwrap(api.GET("/api/v1/updates/rollback")), refetchInterval: 10_000 });
  const request = useMutation({
    mutationFn: (version: string) => unwrap(api.POST("/api/v1/updates/rollback/request", { body: { version } })),
    onSuccess: () => {
      setConfirm(false);
      setRequested(true);
      toast.ok(t("rollback.requested"));
      void qc.invalidateQueries({ queryKey: qk.updates });
    },
    onError: (e) => toast.error(errorText(e)),
  });
  const v = rollback.data;
  const reasons = {
    no_previous: "rollback.noPrevious", schema_changed: "rollback.schemaChanged",
    image_missing: "rollback.imageMissing", version_unsupported: "rollback.unsupported",
    version_changed: "rollback.versionChanged", used: "rollback.used",
  } as const;
  const reason = reasons[v?.reason as keyof typeof reasons] ?? "rollback.noPrevious";
  return <div className="mt-4 border-t border-[var(--line)] pt-4">
    <h3 className="font-semibold">{t("rollback.title")}</h3>
    <p className="mt-1 text-xs text-[var(--ink-500)]">{t("rollback.description")}</p>
    {rollback.isError ? <p className="mt-2 text-xs text-[var(--berry-600)]">{errorText(rollback.error)}</p> : null}
    {v && !v.available ? <p className="mt-2 text-xs text-[var(--ink-500)]">{t(reason)}</p> : null}
    {requested ? <p className="mt-2 text-xs text-[var(--ink-500)]">{t("rollback.waiting")}</p> : null}
    {v?.available ? <Button className="mt-3" disabled={busy || requested} loading={request.isPending} onClick={() => setConfirm(true)}>{t("rollback.button", { v: v.version })}</Button> : null}
    <Confirm open={confirm} onOpenChange={setConfirm} title={t("rollback.confirmTitle", { v: v?.version ?? "" })} text={t("rollback.confirmText")} confirm={t("rollback.confirm")} loading={request.isPending} onConfirm={() => { if (v?.available && !busy && !requested) request.mutate(v.version); }} />
  </div>;
}
