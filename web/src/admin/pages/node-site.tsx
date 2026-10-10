import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Globe, Trash2, Upload } from "lucide-react";
import { useRef, useState } from "react";
import { api, errorText, rawApi, unwrap, type Schemas } from "../../api/client";
import { Drawer } from "../../components/overlay";
import { Button, Field, Pill } from "../../components/ui";
import { t } from "../../i18n";
import { bytes } from "../../lib/format";
import { nodeLabel } from "../../lib/node-label";

// Kinkan: the website a node shows (ROADMAP item 2).

type Node = Schemas["NodeInfo"];
type Site = Schemas["SiteView"];

export function SiteDrawer({ node, onClose }: { node: Node | null; onClose: () => void }) {
  return <Drawer open={!!node} onOpenChange={(v) => !v && onClose()} title={t("site.title")} meta={node ? nodeLabel(node) : undefined} wide>
    {node ? <SiteForm key={node.id} node={node} /> : null}
  </Drawer>;
}

function SiteForm({ node }: { node: Node }) {
  const qc = useQueryClient();
  const sites = useQuery({ queryKey: ["sites"], queryFn: ({ signal }) => unwrap(api.GET("/api/v1/sites", { signal })) });
  const list = sites.data ?? [];
  const current = list.find((s) => s.nodes.includes(node.id));
  const [picked, setPicked] = useState<number | null>(null);
  const choice = picked ?? current?.id ?? 0;
  const [message, setMessage] = useState("");
  const [formError, setFormError] = useState("");
  const refresh = () => Promise.all([qc.invalidateQueries({ queryKey: ["sites"] }), qc.invalidateQueries({ queryKey: ["nodes"] }), qc.invalidateQueries({ queryKey: ["node-site", node.id] })]);

  const assign = useMutation({
    mutationFn: (siteID: number) => unwrap(api.PUT("/api/v1/nodes/{id}/site", { params: { path: { id: node.id } }, body: { site_id: siteID } })),
    onSuccess: async (_, siteID) => { setPicked(null); setMessage(siteID ? t("site.assigned") : t("site.cleared")); await refresh(); },
    onError: (e) => setFormError(errorText(e)),
  });
  const remove = useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE("/api/v1/sites/{id}", { params: { path: { id } } })),
    onSuccess: async () => { setMessage(t("site.deleted")); await refresh(); },
    onError: (e) => setFormError(errorText(e)),
  });

  return <div className="space-y-5 pt-5">
    <p className="text-[13px] text-[var(--ink-500)]">{t("site.intro")}</p>
    <div className="banner info">{t("site.required")}</div>
    <Status node={node} current={current} />
    {sites.isError ? <p role="alert">{errorText(sites.error)}</p> : sites.isPending ? <p role="status">{t("common.loading")}</p> : <>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t("site.choose")}</legend>
        <label className="flex items-center gap-2 rounded-xl border border-[var(--hairline)] p-3 text-sm">
          <input type="radio" name="node-site" checked={choice === 0} onChange={() => setPicked(0)} /> {t("site.none")}
        </label>
        {list.map((s) => <SiteRow key={s.id} site={s} checked={choice === s.id} onPick={() => setPicked(s.id)}
          onDelete={s.nodes.length === 0 ? () => { setFormError(""); setMessage(""); remove.mutate(s.id); } : undefined} deleting={remove.isPending && remove.variables === s.id} />)}
      </fieldset>
      <Button variant="primary" loading={assign.isPending} disabled={choice === (current?.id ?? 0)} onClick={() => { setFormError(""); setMessage(""); assign.mutate(choice); }}>
        <Globe size={16} aria-hidden />{t("site.apply")}
      </Button>
    </>}
    <UploadForm onUploaded={async (existing) => { setMessage(existing ? t("site.existing") : t("site.uploaded")); setFormError(""); await refresh(); }} onError={(e) => { setMessage(""); setFormError(e); }} />
    {message ? <p role="status" aria-live="polite" className="text-sm">{message}</p> : null}
    {formError ? <p role="alert" className="text-sm text-[var(--berry-600)]">{formError}</p> : null}
  </div>;
}

// What the node itself says it serves: the panel's choice reaches it within seconds.
function Status({ node, current }: { node: Node; current?: Site }) {
  const result = useQuery({
    queryKey: ["node-site", node.id],
    queryFn: ({ signal }) => unwrap(api.GET("/api/v1/nodes/{id}/site", { params: { path: { id: node.id } }, signal })),
    refetchInterval: 5_000,
  });
  const served = result.data?.served;
  if (!current && !served) return <p className="text-sm">{t("site.statusNone")}</p>;
  if (served?.error) return <div className="banner err">{t("site.statusError", { error: served.error })}</div>;
  if (current && served?.hash === current.hash) {
    return <div className="banner info">{t("site.statusServed")}
      <span className="mono block text-xs">{[served.http, served.https].filter(Boolean).join(" · ")}</span>
    </div>;
  }
  if (current && node.status === "ok") return <div className="banner warn">{t("site.statusPending")}</div>;
  return null;
}

function SiteRow({ site, checked, onPick, onDelete, deleting }: { site: Site; checked: boolean; onPick: () => void; onDelete?: () => void; deleting: boolean }) {
  return <div className="flex items-start gap-3 rounded-xl border border-[var(--hairline)] p-3">
    <input type="radio" name="node-site" checked={checked} onChange={onPick} aria-label={site.name} className="mt-1" />
    <div className="min-w-0 flex-1 space-y-1 text-sm">
      <div className="font-medium break-words">{site.name}</div>
      {site.title && site.title !== site.name ? <div className="break-words text-[var(--ink-500)]">{site.title}</div> : null}
      <div className="text-xs text-[var(--ink-500)]">{t("site.meta", { files: site.files, size: bytes(site.size), nodes: site.nodes.length })}</div>
      <div className="flex flex-wrap gap-2">
        {site.several_nodes ? <Pill tone="warn">{t("site.severalNodes")}</Pill> : null}
        {site.same_title ? <Pill tone="warn">{t("site.sameTitle")}</Pill> : null}
      </div>
    </div>
    {onDelete ? <Button size="sm" loading={deleting} onClick={onDelete} aria-label={t("site.delete", { name: site.name })}><Trash2 size={16} aria-hidden /></Button> : null}
  </div>;
}

function UploadForm({ onUploaded, onError }: { onUploaded: (existing: boolean) => void; onError: (text: string) => void }) {
  const file = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [picked, setPicked] = useState<File | null>(null);
  const upload = useMutation({
    mutationFn: (f: File) => rawApi(`/api/v1/sites${name.trim() ? `?name=${encodeURIComponent(name.trim())}` : ""}`, { method: "POST", body: f, headers: { "Content-Type": "application/zip" } }),
    onSuccess: (r: { existing?: boolean }) => { setName(""); setPicked(null); if (file.current) file.current.value = ""; onUploaded(!!r.existing); },
    onError: (e) => onError(errorText(e)),
  });
  return <div className="space-y-3 rounded-xl border border-[var(--hairline)] p-4">
    <div className="text-sm font-medium">{t("site.upload")}</div>
    <p className="text-xs text-[var(--ink-500)]">{t("site.uploadHint")}</p>
    <Field label={t("site.file")} htmlFor="site-file"><input id="site-file" ref={file} className="input" type="file" accept=".zip,application/zip" onChange={(e) => setPicked(e.target.files?.[0] ?? null)} /></Field>
    <Field label={t("site.name")} htmlFor="site-name" hint={t("site.nameHint")}><input id="site-name" className="input" type="text" maxLength={100} value={name} onChange={(e) => setName(e.target.value)} /></Field>
    <Button loading={upload.isPending} disabled={!picked} onClick={() => picked && upload.mutate(picked)}><Upload size={16} aria-hidden />{t("site.uploadButton")}</Button>
  </div>;
}
