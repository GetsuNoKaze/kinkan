import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorText, unwrap } from "../../../api/client";
import { meQuery, qk } from "../../../api/hooks";
import { Confirm } from "../../../components/overlay";
import { Button, Field } from "../../../components/ui";
import { useToast } from "../../../components/toast";
import { t } from "../../../i18n";

export function KinkanRollback({ busy }: { busy: boolean }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [confirm, setConfirm] = useState(false);
  const [requested, setRequested] = useState(false);
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const me = useQuery(meQuery);
  const totp = !!me.data?.admin.totp_enabled;
  const rollback = useQuery({ queryKey: ["kinkan-rollback"], queryFn: () => unwrap(api.GET("/api/v1/updates/rollback")), refetchInterval: 10_000 });
  const request = useMutation({
    mutationFn: (version: string) => unwrap(api.POST("/api/v1/updates/rollback/request", { body: { version, password, totp: code || undefined } })),
    onSuccess: () => {
      setConfirm(false);
      setPassword("");
      setCode("");
      setRequested(true);
      toast.ok(t("rollback.requested"));
      void qc.invalidateQueries({ queryKey: qk.updates });
    },
    onError: (e) => {
      setConfirm(false);
      toast.error(errorText(e));
    },
  });
  const v = rollback.data;
  const reasons = {
    no_previous: "rollback.noPrevious", schema_changed: "rollback.schemaChanged",
    image_missing: "rollback.imageMissing", version_unsupported: "rollback.unsupported",
    version_changed: "rollback.versionChanged", used: "rollback.used",
  } as const;
  const ready = !!password && (!totp || code.length === 6);
  const reason = reasons[v?.reason as keyof typeof reasons] ?? "rollback.noPrevious";
  return <div className="mt-4 border-t border-[var(--line)] pt-4">
    <h3 className="font-semibold">{t("rollback.title")}</h3>
    <p className="mt-1 text-xs text-[var(--ink-500)]">{t("rollback.description")}</p>
    {rollback.isError ? <p className="mt-2 text-xs text-[var(--berry-600)]">{errorText(rollback.error)}</p> : null}
    {v && !v.available ? <p className="mt-2 text-xs text-[var(--ink-500)]">{t(reason)}</p> : null}
    {requested ? <p className="mt-2 text-xs text-[var(--ink-500)]">{t("rollback.waiting")}</p> : null}
    {v?.available && !requested ? <div className="mt-3 flex flex-col gap-3">
      <Field label={t("rollback.password")} htmlFor="rb-pw" hint={t("rollback.passwordHint")}>
        <input id="rb-pw" type="password" className="input max-w-[320px]" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
      </Field>
      {totp ? <Field label={t("settings.totpCode")} htmlFor="rb-totp">
        <input id="rb-totp" className="input mono max-w-[160px] tracking-[0.2em]" inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.trim())} autoComplete="one-time-code" />
      </Field> : null}
      <div><Button disabled={busy || !ready} loading={request.isPending} onClick={() => setConfirm(true)}>{t("rollback.button", { v: v.version })}</Button></div>
    </div> : null}
    <Confirm open={confirm} onOpenChange={setConfirm} title={t("rollback.confirmTitle", { v: v?.version ?? "" })} text={t("rollback.confirmText")} confirm={t("rollback.confirm")} loading={request.isPending} onConfirm={() => { if (v?.available && !busy && !requested && ready) request.mutate(v.version); }} />
  </div>;
}
