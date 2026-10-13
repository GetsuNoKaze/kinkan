import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, errorText, unwrap, type Inbound } from "../../../api/client";
import { qk } from "../../../api/hooks";
import { useToast } from "../../../components/toast";
import { Button, Field } from "../../../components/ui";
import { t } from "../../../i18n";

// How many TLS connections TrustTunnel clients may keep to this inbound. Saved on its own:
// it rewrites the template, so it waits while the config tab has unsaved edits.
export function KinkanTTConnections({ inbound, locked }: { inbound: Inbound; locked: boolean }) {
  const qc = useQueryClient();
  const toast = useToast();
  const tt = inbound.type === "trusttunnel";
  const key = ["kinkan-tt-connections", inbound.id];
  const current = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => unwrap(api.GET("/api/v1/inbounds/{id}/tt-connections", { params: { path: { id: inbound.id } }, signal })),
    enabled: tt,
  });
  const [value, setValue] = useState(1);
  useEffect(() => {
    if (current.data) setValue(current.data.value);
  }, [current.data]);
  const save = useMutation({
    mutationFn: (v: number) => unwrap(api.PUT("/api/v1/inbounds/{id}/tt-connections", { params: { path: { id: inbound.id } }, body: { value: v } })),
    onSuccess: (data) => {
      qc.setQueryData(key, data);
      void qc.invalidateQueries({ queryKey: qk.inbounds });
      toast.ok(t("ttConnections.saved"));
    },
    onError: (e) => toast.error(errorText(e)),
  });
  if (!tt || !current.data) return null;
  const { max, default: def } = current.data;
  const changed = value !== current.data.value;
  return (
    <Field label={t("ttConnections.label")} htmlFor="ed-tt-conns" hint={locked ? t("inbounds.fingerprintLocked") : t("ttConnections.hint", { def })}>
      <div className="flex items-center gap-2">
        <select id="ed-tt-conns" className="input max-w-[120px]" value={value} disabled={locked || save.isPending} onChange={(e) => setValue(Number(e.target.value))}>
          {Array.from({ length: max }, (_, i) => i + 1).map((n) => (
            <option key={n} value={n}>
              {n === def ? t("ttConnections.default", { n }) : n}
            </option>
          ))}
        </select>
        <Button size="sm" disabled={!changed || locked} loading={save.isPending} onClick={() => save.mutate(value)}>
          {t("ttConnections.apply")}
        </Button>
      </div>
    </Field>
  );
}
