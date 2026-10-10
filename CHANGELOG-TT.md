# Kinkan changelog

Fork release notes. Upstream history remains in CHANGELOG.md.

## Unreleased
### en
- `kinkan probe` checks the unauthenticated TrustTunnel surface (33 checks), compares cover responses and the HTTP/2 fingerprint against an optional HTTPS reference, and reports explicit proxy leaks separately from differences and incomplete checks. Text/JSON output; works on panels and separate nodes. See PROBE-TT.md.
- The kinkan command: `kinkan update`, `kinkan status` and the rest; mikan keeps working, kinkan is a link to it. Servers get it on install or after an update.
- Kinkan's releases (vX-tt.N) are stable releases now: kinkan update and the daily update check take them, GitHub marks them latest and the one-line install works. Builds with more after -tt.N (vX-tt.N-rc.1) stay pre-releases on the beta channel.
- A server on 0.5.0.5-tt.3 or earlier does not take -tt.N as stable yet: update it once with this release's installer, as in RELEASE-TT.md; later releases arrive by kinkan update.
### ru
- `kinkan probe`: 33 проверки TrustTunnel без пароля, сравнение ответов и HTTP/2 отпечатка с HTTPS cover-сайтом. Явные признаки прокси, отличия и невыполненные проверки разделены; текстовый/JSON отчёт. Работает на панели и отдельной ноде. Инструкция — PROBE-TT.md.
- Команда kinkan: `kinkan update`, `kinkan status` и остальные; mikan по-прежнему работает, kinkan — ссылка на неё. Серверы получают её при установке или после обновления.
- Релизы Kinkan (vX-tt.N) теперь стабильные: их берут kinkan update и ежедневная проверка обновлений, GitHub отмечает их как latest, работает установка одной строкой. Сборки с чем-то после -tt.N (vX-tt.N-rc.1) остаются пре-релизами в канале beta.
- Сервер на 0.5.0.5-tt.3 или раньше ещё не считает -tt.N стабильными: обновите его один раз установщиком этого релиза, как в RELEASE-TT.md; дальше обновления приходят через kinkan update.

## 0.5.0.5-tt.3
### en
- The fork is now called Kinkan: repository GetsuNoKaze/kinkan, image ghcr.io/getsunokaze/kinkan. On a server it keeps Mikan's names (the mikan command, /opt/mikan) and releases keep the -tt.N suffix. Servers on an earlier fork release update from the new repository; GitHub redirects the old address.
- ROADMAP.md lists what comes next.
- New release signing key. Servers on 0.5.0.5-tt.1 or tt.2 trust the old key and do not take updates signed with the new one: reinstall them from this release.
### ru
- Форк теперь называется Kinkan: репозиторий GetsuNoKaze/kinkan, образ ghcr.io/getsunokaze/kinkan. На сервере остаются имена Mikan (команда mikan, /opt/mikan), у релизов остаётся суффикс -tt.N. Серверы на прежних релизах форка обновляются из нового репозитория; старый адрес GitHub перенаправляет.
- План — в ROADMAP.md.
- Новый ключ подписи релизов. Серверы на 0.5.0.5-tt.1 и tt.2 доверяют старому ключу и обновления с новой подписью не примут: их нужно переустановить из этого релиза.

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
