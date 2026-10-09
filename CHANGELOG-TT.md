# Mikan TT changelog

Fork release notes. Upstream history remains in CHANGELOG.md.

## 0.5.0.5-tt.1
### en
- First Mikan TT release candidate, based on Mikan 0.5.0.4.
- TrustTunnel accepts a plain HTTP fallback site for requests without valid credentials, including CONNECT, and advertises h2 and http/1.1 when fallback is configured.
- Fallback requests have timeout and concurrency limits and are cancelled when the listener closes.
- The panel accepts fallback in TrustTunnel templates. Both the panel and nodes must use this build.
- Releases, installer downloads and signed updates use the fork's repository and signing key. Migration from official Mikan is manual; its updater does not trust this key.
- The cover site still uses Caddy. Add a loopback HTTP listener for TrustTunnel; keep the HTTPS listener and certificates for REALITY.
### ru
- Первый кандидат в релизы Mikan TT на основе Mikan 0.5.0.4.
- TrustTunnel направляет запросы без правильных учётных данных, включая CONNECT, на HTTP cover-сайт. При настроенном fallback согласует h2 и http/1.1.
- Для fallback действуют таймауты и предел одновременных запросов; при закрытии listener запросы отменяются.
- Панель принимает fallback в шаблоне TrustTunnel. И панель, и ноды должны использовать эту сборку.
- Релизы, установщик и подписанные обновления используют репозиторий и ключ форка. Переход с официального Mikan выполняется вручную: его обновлятор не доверяет этому ключу.
- Cover-сайт по-прежнему обслуживает Caddy. Для TrustTunnel нужен локальный HTTP listener; HTTPS и сертификаты для REALITY сохраняются.
