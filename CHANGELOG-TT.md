# Mikan TT changelog

Fork release notes. Upstream history remains in CHANGELOG.md.

## 0.5.0.5-tt.2
### en
- With fallback set, TrustTunnel serves HTTP/2 only to clients that negotiated h2 over ALPN. A prober sending the HTTP/2 preface over TLS that negotiated http/1.1 or nothing used to get HTTP/2 frames back, which no ordinary HTTPS site does; it now gets an HTTP/1 answer from the fallback. mihomo's client always offers h2; a client that skips ALPN cannot use an inbound with fallback.
- A request with wrong credentials is logged again when the fallback serves it, so a misconfigured client shows in the log. Requests without credentials are logged at debug level only.
- TrustTunnel compares passwords in constant time.
- The panel checks fallback when a template is saved: host:port with a port from 1 to 65535, no scheme or path. Link-local and unspecified addresses are refused, since unauthenticated requests from anywhere would reach the cloud metadata service (169.254.169.254) through the inbound.
- The sync workflow keeps one sync/upstream branch and updates its open pull request instead of opening one per upstream commit. The fork's workflows pin actions by commit, like the rest of the repository. With the SYNC_TOKEN secret the sync pull request also runs mikan's full CI.
### ru
- При заданном fallback TrustTunnel отдаёт HTTP/2 только клиентам, согласовавшим h2 через ALPN. Раньше на преамбулу HTTP/2 поверх TLS с http/1.1 или без ALPN сервер отвечал кадрами HTTP/2, чего обычный HTTPS-сайт не делает; теперь такой запрос получает HTTP/1-ответ от fallback. Клиент mihomo всегда предлагает h2; клиент без ALPN не сможет пользоваться inbound с fallback.
- Запрос с неверными учётными данными снова пишется в лог, когда его обслуживает fallback, — неверно настроенного клиента видно. Запросы без учётных данных пишутся только на уровне debug.
- TrustTunnel сравнивает пароли за постоянное время.
- Панель проверяет fallback при сохранении шаблона: host:port, порт от 1 до 65535, без схемы и пути. Link-local и нулевые адреса запрещены: иначе запросы без пароля из интернета доходили бы через inbound до сервиса метаданных облака (169.254.169.254).
- Синхронизация держит одну ветку sync/upstream и обновляет её открытый PR, а не открывает новый на каждый коммит апстрима. Workflow форка закрепляют actions по коммиту, как остальной репозиторий. С секретом SYNC_TOKEN на PR синхронизации запускается и полный CI mikan.

## 0.5.0.5-tt.1
### en
- First Mikan TT release candidate, based on Mikan 0.5.0.4.
- Build with Go 1.27.2 and golang.org/x/net 0.60.0, including the current HTTP/2 security fixes.
- TrustTunnel accepts a plain HTTP fallback site for requests without valid credentials, including CONNECT, and advertises h2 and http/1.1 when fallback is configured.
- Fallback requests have timeout and concurrency limits and are cancelled when the listener closes.
- The panel accepts fallback in TrustTunnel templates. Both the panel and nodes must use this build.
- Releases, installer downloads and signed updates use the fork's repository and signing key. Migration from official Mikan is manual; its updater does not trust this key.
- The cover site still uses Caddy. Add a loopback HTTP listener for TrustTunnel; keep the HTTPS listener and certificates for REALITY.
### ru
- Первый кандидат в релизы Mikan TT на основе Mikan 0.5.0.4.
- Сборка на Go 1.27.2 и golang.org/x/net 0.60.0 с актуальными исправлениями безопасности HTTP/2.
- TrustTunnel направляет запросы без правильных учётных данных, включая CONNECT, на HTTP cover-сайт. При настроенном fallback согласует h2 и http/1.1.
- Для fallback действуют таймауты и предел одновременных запросов; при закрытии listener запросы отменяются.
- Панель принимает fallback в шаблоне TrustTunnel. И панель, и ноды должны использовать эту сборку.
- Релизы, установщик и подписанные обновления используют репозиторий и ключ форка. Переход с официального Mikan выполняется вручную: его обновлятор не доверяет этому ключу.
- Cover-сайт по-прежнему обслуживает Caddy. Для TrustTunnel нужен локальный HTTP listener; HTTPS и сертификаты для REALITY сохраняются.
