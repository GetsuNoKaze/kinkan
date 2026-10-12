import { t } from "../../i18n";

// What a JA4 fingerprint says about the program behind it, from its readable part
// ("t13d1516h2": TLS 1.3, a site name, 15 ciphers, 16 extensions, ALPN h2). These are
// hints, not proof: a program can copy a browser's ClientHello (uTLS does).
export function tlsKind(ja4: string): string {
  const m = /^t(\d\d|s3)([di])(\d\d)(\d\d)(..)_/.exec(ja4);
  if (!m) return t("scanners.tls.other");
  const [, version, sni, ciphers, exts, alpn] = m;
  if (sni === "i") return t("scanners.tls.noName");
  if (version !== "13") return t("scanners.tls.oldTLS");
  if (alpn === "00") return t("scanners.tls.noALPN");
  if (Number(ciphers) >= 14 && Number(exts) >= 14 && alpn === "h2") return t("scanners.tls.browserLike");
  return t("scanners.tls.program");
}

// The fingerprints shared by the most addresses: one program behind many IPs is a scan
// through a proxy network, not many people.
export function sharedFingerprints(rows: { ip: string; tls?: string; count: number; client: boolean; own?: boolean }[], top = 3) {
  const by = new Map<string, { ips: Set<string>; count: number }>();
  for (const r of rows) {
    if (!r.tls || r.client || r.own) continue;
    const v = by.get(r.tls) ?? { ips: new Set<string>(), count: 0 };
    v.ips.add(r.ip);
    v.count += r.count;
    by.set(r.tls, v);
  }
  return [...by.entries()].map(([tls, v]) => ({ tls, ips: v.ips.size, count: v.count })).filter((v) => v.ips > 1).sort((a, b) => b.ips - a.ips || b.count - a.count).slice(0, top);
}
