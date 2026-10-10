# Kinkan changelog

Fork release notes. Upstream history remains in CHANGELOG.md.

## 0.5.0.5-tt.6
### en
- The panel's check now covers all of a node's inbounds, not only TrustTunnel (Nodes → the check, and `kinkan probe --protocol` / `--inbounds`), with one report per inbound. What it does not cover yet is listed in PROBE-PROTOCOLS.md.
- Scanner journal: a node records who was refused on its inbounds — address, network, country and confirmed reverse DNS, the inbound and the reason, counted per day — never credentials, URLs or users' traffic. The panel shows it as a table and a chart with spikes and marks addresses its own clients use; 30 days, at most 10,000 records per node.
- The node's own site announces Caddy's header limit over HTTP/2 (MAX_HEADER_LIST_SIZE 16704), as the TrustTunnel front does: checked on the test node, REALITY's own site and the TrustTunnel port now give the same HTTP/2 fingerprint.
### ru
- Проверка в панели охватывает все подключения ноды, а не только TrustTunnel (Ноды → проверка, `kinkan probe --protocol` / `--inbounds`), с отчётом по каждому. Что ещё не покрыто — в PROBE-PROTOCOLS.md.
- Журнал сканеров: нода записывает, кому было отказано на её подключениях — адрес, сеть, страну и подтверждённый обратный DNS, подключение и причину, по дням; без паролей, URL и трафика пользователей. В панели — таблица и график со всплесками, адреса своих клиентов помечаются; 30 дней, не больше 10 000 записей на ноду.
- Сайт ноды объявляет по HTTP/2 тот же лимит заголовков, что Caddy (MAX_HEADER_LIST_SIZE 16704), как и фронт TrustTunnel: на тестовой ноде сайт за REALITY и порт TrustTunnel теперь дают одинаковый отпечаток HTTP/2.

## 0.5.0.5-tt.5
### en
- Node site: upload a static website as a zip in the panel (Nodes → Node site) and give it to nodes. The panel checks the archive (index.html, 404.html, robots.txt and a favicon; static files only; no parent paths, links or zip bombs), keeps it in the database and sends it to the node, which checks it again and serves it on 127.0.0.1 like Caddy's file_server — over HTTP on 17080 and over TLS with the node's certificate on 17443. Once the node serves it, TrustTunnel without a fallback of its own falls back to it, and REALITY aimed at the panel's own HTTPS aims at the site instead of the panel's empty 404. REALITY may also be aimed at the site (127.0.0.1:17443) by hand on any node that has one. Mikan's nodes ignore all of it. The panel warns when one site is on several nodes: it ties them together.
- The fork's database migrations have their own goose table (kinkan_db_version), apart from Mikan's.
- The kinkan command is also made when the one-line install updates an installed server; 0.5.0.5-tt.4 made it only on a fresh install or a later `mikan update`.
### ru
- Сайт ноды: статический сайт загружается в панель zip-архивом (Ноды → «Сайт ноды») и назначается нодам. Панель проверяет архив (index.html, 404.html, robots.txt и favicon; только статика; без путей наверх, ссылок и zip-бомб), хранит его в базе и отправляет ноде; нода проверяет его ещё раз и отдаёт на 127.0.0.1 как file_server Caddy — по HTTP на 17080 и по TLS своим сертификатом на 17443. Когда нода отдаёт сайт, TrustTunnel без своего fallback смотрит на него, а REALITY, направленный на HTTPS панели, — на сайт вместо пустого 404 панели. На любой ноде с сайтом REALITY можно направить на него и вручную (127.0.0.1:17443). Ноды Mikan всего этого не видят. Панель предупреждает, если один сайт стоит на нескольких нодах: это их связывает.
- У миграций базы форка своя таблица goose (kinkan_db_version), отдельно от Mikan.
- Команда kinkan появляется и тогда, когда установка одной строкой обновляет уже установленный сервер; в 0.5.0.5-tt.4 она появлялась только при новой установке или при следующем `mikan update`.

## 0.5.0.5-tt.4
### en
- TrustTunnel fallback: one client network (an IPv4 address, an IPv6 /64) holds at most 64 of the 256 fallback requests, so one slow prober cannot turn the cover site into a blank 503 for everyone. The backend address is checked when dialing too: a host name that resolves to a link-local (cloud metadata), multicast or unspecified address is refused. Wrong credentials are logged at most once per 10 seconds, with the number of the rest.
- The TrustTunnel fallback's HTTP/2 now matches Caddy in two more ways, measured against Caddy 2.11.7: MAX_HEADER_LIST_SIZE 16704 instead of Go's 1 MB default, and Date set by the front and sent last instead of the backend's sorted among the headers. The third difference, SETTINGS 9 (NO_RFC7540_PRIORITIES), is fixed in the fork's own copy of metacubex/http (third_party/metacubex-http), so the SETTINGS frame now matches Caddy's.
- `kinkan probe --address IP` connects to that IP and keeps the URLs' names for SNI and Host.
- The scanner (36 checks): an absolute-form request as to an open proxy, a foreign Host, and the HTTP/1 answer to a bare HTTP/2 preface compared with the cover. An unreachable reference is one ERROR instead of a timeout per comparison.
- Check TrustTunnel in the admin UI: a verdict line, worst results first, filters by level, the checked IP and start time, a warning for a node on the panel's own server, and the HTTP/2 fingerprint difference as a table. The cover port is blank by default. A second check while one runs says so instead of the REALITY target search message.
- Admin UI: Nodes → Check TrustTunnel runs the scanner from the panel server, compares a chosen inbound with the same node's HTTPS cover port, and displays/downloads the report. Session-only, CSRF-protected, public DNS pinned for every transport; one scan at a time, with a 75-second deadline and partial results on timeout.
- `kinkan probe` checks the unauthenticated TrustTunnel surface (33 checks), compares cover responses and the HTTP/2 fingerprint against an optional HTTPS reference, and reports explicit proxy leaks separately from differences and incomplete checks. Text/JSON output; works on panels and separate nodes. See PROBE-TT.md.
- The kinkan command: `kinkan update`, `kinkan status` and the rest; mikan keeps working, kinkan is a link to it. Servers get it on install or after an update.
- Kinkan's releases (vX-tt.N) are stable releases now: kinkan update and the daily update check take them, GitHub marks them latest and the one-line install works. Builds with more after -tt.N (vX-tt.N-rc.1) stay pre-releases on the beta channel.
- A server on 0.5.0.5-tt.3 or earlier does not take -tt.N as stable yet: update it once with this release's installer, as in RELEASE-TT.md; later releases arrive by kinkan update.
### ru
- Fallback TrustTunnel: одна сеть клиента (адрес IPv4, IPv6 /64) занимает не больше 64 из 256 запросов fallback, и один медленный сканер не превращает сайт-прикрытие в пустой 503 для всех. Адрес бэкенда проверяется и при подключении: имя, которое указывает на link-local (метаданные облака), multicast или нулевой адрес, отклоняется. Неверный пароль пишется в лог не чаще раза в 10 секунд, с числом пропущенных.
- HTTP/2 у fallback TrustTunnel ещё в двух местах совпадает с Caddy (сверено с Caddy 2.11.7): MAX_HEADER_LIST_SIZE 16704 вместо 1 МБ по умолчанию в Go, и Date ставит сам фронт, последним, а не Date бэкенда среди остальных заголовков. Третье отличие, SETTINGS 9 (NO_RFC7540_PRIORITIES), исправлено в собственной копии metacubex/http (third_party/metacubex-http): кадр SETTINGS теперь совпадает с Caddy.
- `kinkan probe --address IP` подключается к этому IP, а SNI и Host берёт из адресов.
- Сканер (36 проверок): запрос в абсолютной форме как к открытому прокси, чужой Host, ответ HTTP/1 на голую преамбулу HTTP/2 сравнивается с эталоном. Недоступный эталон — одна строка ERROR вместо таймаута на каждое сравнение.
- Проверка TrustTunnel в админке: итоговая строка, худшие результаты сверху, фильтры по уровню, проверенный IP и время, предупреждение для ноды на сервере панели, различия отпечатка HTTP/2 таблицей. Порт эталона по умолчанию пустой. Повторный запуск во время проверки сообщает об этом, а не текстом подбора целей REALITY.
- В админке: Ноды → Проверка TrustTunnel. Панель проверяет выбранное подключение и HTTPS cover-порт той же ноды, показывает отчёт и даёт скачать JSON. Только админская сессия с CSRF; публичный IP закрепляется на весь запуск. Одна проверка за раз, до 75 секунд, при таймауте — частичный отчёт.
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
