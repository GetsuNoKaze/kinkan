# Kinkan changelog

Fork release notes. Upstream history remains in CHANGELOG.md.

## 0.5.2.1-tt.1
Stable, on Mikan 0.5.2.0 (the 0.5.1.1 betas released). From here on Kinkan follows Mikan's stable releases, not every candidate. After the panel, update the nodes (Nodes → Update all): the new core and much of the list below work only on updated nodes.
### en
- Merge Mikan 0.5.2.0, after its betas 0.5.1.1-rc.11 to rc.15: server groups and what a plan includes, Shadowsocks-2022 with a key per client, the core's own speed limits, the shared port 443, the node API for the panel alone, the front page for strangers, the activity log, the panel as an app, and the nodes' core from Citrus ca8204d9. Mikan's release notes list everything; read "Update the nodes together with the panel" and "Shadowsocks-2022 (AES): refresh the subscriptions" there before updating.
- The marketplace is checked with its own keys again, as Mikan now signs its catalog and the community registry apart from its releases; the fork's copy of Mikan's release key for the catalog is gone. Kinkan's releases are signed by Kinkan's key alone, the only one in `ReleaseKeys`, the installer and `install.sh`.
- The fork's patches to the core (the JA4 fingerprint in REALITY auth events, CONNECT without credentials behind a fallback) apply on Citrus ca8204d9 unchanged.
- The fork's buttons and update notice on a node's card follow the card to its own file (`nodes/card.tsx`).
- REALITY on the node's own site (`127.0.0.1:17443`) passes the target checks: `mikan admin targets check`/`apply`, the form's Check button (it now says which node) and autotune's SNI check, all done by the node itself. Before, the panel refused it as an internal address and the node's check could not dial it, so autotune took a working site for a dead one and moved the inbound to a neighbour's site; clients with the old subscription then failed until they refreshed it.
- Installer: rustls-webpki 0.103.15 → 0.103.16 (RUSTSEC-2026-0382) and webpki-roots 1.0.9 → 1.0.10 (RUSTSEC-2026-0381): distrusted intermediate certificates are no longer accepted.
- The fork's own fixes of 18.10 to the installer (`config.json` read before `.env` after an update that stopped half way, secrets handed over only as root, `test-upgrade.sh` on `config.json`) give way to Mikan's, which 0.5.2.0 ships; the fork keeps only its rollback point in the upgrade test.
### ru
- Слит Mikan 0.5.2.0 — выпущенные беты 0.5.1.1-rc.11–rc.15: группы серверов и состав тарифа, Shadowsocks-2022 с ключом на клиента, ограничение скорости в самом ядре, общий порт 443, API ноды только для панели, страница для чужих, журнал действий, панель как приложение, ядро нод — Citrus ca8204d9. Полный список — в заметках Mikan; перед обновлением прочитайте там «Update the nodes together with the panel» и «Shadowsocks-2022 (AES): refresh the subscriptions».
- Маркетплейс снова проверяется своими ключами: Mikan теперь подписывает каталог и реестр сообщества отдельно от релизов, поэтому копия релизного ключа Mikan для каталога в форке больше не нужна. Релизы Kinkan подписаны только ключом Kinkan — он единственный в `ReleaseKeys`, установщике и `install.sh`.
- Патчи форка к ядру (отпечаток JA4 в событиях REALITY, CONNECT без учётных данных за fallback) ложатся на Citrus ca8204d9 без изменений.
- Кнопки форка и сообщение об обновлении в карточке ноды переехали вместе с карточкой в отдельный файл (`nodes/card.tsx`).
- REALITY на собственном сайте ноды (`127.0.0.1:17443`) проходит проверки цели: `mikan admin targets check`/`apply`, кнопка «Проверить» в форме (теперь передаёт ноду) и проверку автоподбора SNI — всё силами самой ноды. Раньше панель отвергала такую цель как внутренний адрес, а проверка на ноде не могла до неё дозвониться, и автоподбор принимал живой сайт за упавший и переставлял подключение на сайт соседа; клиенты со старой подпиской после этого не подключались, пока не обновят её.
- Установщик: rustls-webpki 0.103.15 → 0.103.16 (RUSTSEC-2026-0382) и webpki-roots 1.0.9 → 1.0.10 (RUSTSEC-2026-0381): недоверенные промежуточные сертификаты больше не принимаются.
- Собственные исправления установщика от 18.10 (`config.json` читается раньше `.env` после прерванного обновления, секреты передаются владельцу только от root, `test-upgrade.sh` на `config.json`) уступили место исправлениям Mikan, вошедшим в 0.5.2.0; в тесте обновления форк оставил только свою точку отката.

## 0.5.1.1-tt.1-rc.1
Beta: the beta channel only. Servers on the stable channel stay on 0.5.0.5-tt.13 until Mikan 0.5.1.1 is released and Kinkan follows it as 0.5.1.1-tt.1.
### en
- Merge Mikan 0.5.1.1-rc.10: nodes run on Mikan's core, Citrus (`getmikan/Citrus`), and the HTTP server comes from Mikan's `getmikan/http`. What is checked and what is not is in BETA-READINESS.md.
- From Mikan rc.9 and rc.10: the node's region, asked over IPv4 and IPv6; the apps' own XHTTP upload; "Check the certificate" checks the subscription domain from outside; the node API waits for the saved state; the Update button names the release it shows.
- Installer: when both `config.json` and `.env` are there (an update that stopped half way), `config.json` is read. Before, `.env` won, and a retried move from SQLite to PostgreSQL made a new database password.
- The node's site comes back after a node restart, with the saved state.
- The TrustTunnel fallback and the auth events are Citrus's own now, so the fork no longer patches them. The JA4 fingerprint in REALITY auth events stays a fork patch.
- Behind a fallback, Citrus does not report requests without credentials, since they are cover-site visits. The fork reports a CONNECT without credentials, which no browser sends to a site, so the scanner journal still counts proxy scans.
- The TrustTunnel front announces the same 16 KiB header limit as the node's site front and Caddy (HTTP/2 SETTINGS `MAX_HEADER_LIST_SIZE`); before, it announced Go's 1 MB.
- The wrong-key probe no longer covers VMess: the core has none, and Mikan turns VMess inbounds into VLESS.
- Backport Go 1.27.2's HTTP/2 HPACK encoder ownership fix (GO-2026-6617) to the bundled HTTP server (now on top of `getmikan/http`). Peer table-size settings are applied after the active frame writer finishes; a regression test covers the deferred update and HPACK size sequence.
- Installer: update rustls from 0.23.45 to 0.23.46, following upstream's security update.
- Require Go 1.27.2 or newer for manual builds as well as the release image, to include the standard library's security fixes.
### ru
- Слит Mikan 0.5.1.1-rc.10: ноды работают на ядре Mikan — Citrus (`getmikan/Citrus`), HTTP-сервер взят из `getmikan/http`. Что проверено, а что нет, — в BETA-READINESS.md.
- Из Mikan rc.9 и rc.10: регион ноды по IPv4 и IPv6; собственная отправка данных XHTTP у приложений; «Проверить сертификат» проверяет домен подписки снаружи; API ноды ждёт восстановления сохранённого состояния; кнопка обновления называет релиз, который показывает.
- Установщик: если есть и `config.json`, и `.env` (обновление остановилось на полпути), читается `config.json`. Раньше побеждал `.env`, и повтор переезда SQLite → PostgreSQL создавал новый пароль базы.
- Сайт ноды возвращается после перезапуска ноды вместе с сохранённым состоянием.
- Fallback TrustTunnel и события авторизации теперь есть в самом Citrus, форк их больше не патчит. Отпечаток JA4 в событиях REALITY остаётся патчем форка.
- За fallback Citrus не сообщает о запросах без учётных данных: это посетители сайта-прикрытия. Форк сообщает о CONNECT без учётных данных (браузер не шлёт его на сайт), поэтому журнал сканеров по-прежнему видит прокси-сканы.
- Фронт TrustTunnel объявляет тот же лимит заголовков 16 КиБ, что сайт ноды и Caddy (HTTP/2 SETTINGS `MAX_HEADER_LIST_SIZE`); раньше объявлял 1 МБ, как Go по умолчанию.
- Проба неверного ключа больше не проверяет VMess: в ядре его нет, Mikan переводит такие inbound в VLESS.
- В копию HTTP-сервера (теперь поверх `getmikan/http`) перенесено исправление гонки HPACK из Go 1.27.2 (GO-2026-6617). Настройки размера таблицы применяются после завершения текущей записи кадра; регрессионный тест проверяет отложенное обновление и последовательность размеров HPACK.
- Установщик: rustls обновлён с 0.23.45 до 0.23.46 по исправлению безопасности upstream.
- Для ручной сборки тоже требуется Go 1.27.2 или новее, с исправлениями безопасности стандартной библиотеки.

## 0.5.0.5-tt.13
### en
- TrustTunnel: the admin chooses how many connections clients may keep to an inbound (1 to 8, default 1), on the inbound's page; subscriptions carry it as `max-connections`. Clients with the fix in getmikan/Citrus#54 open at most that many TLS connections; older clients limit only their pool and can still open several connections at a cold start.
- REALITY: a server name that belongs to the panel or to another node is refused when the inbound is saved. The target on this node answers with its own certificate, so clients sending that name never connect; this is how VLESS stopped working on two nodes on 12.10.
- Mikan's NOTICE and TRADEMARK are followed: the panel credits Mikan (Copyright (C) 2026 Miroshka and Mikan), says that Kinkan is a modified version, and no longer shows the name mikan. NOTICE.md and TRADEMARK.md are Mikan's, NOTICE-KINKAN.md is the fork's.
### ru
- TrustTunnel: админ выбирает, сколько соединений клиенты могут держать к подключению (от 1 до 8, по умолчанию 1), на странице подключения; подписка передаёт это как `max-connections`. Клиенты с правкой из getmikan/Citrus#54 открывают не больше стольких TLS-соединений; старые клиенты ограничивают только свой пул и при холодном старте всё ещё могут открыть несколько соединений.
- REALITY: имя сервера, которое принадлежит панели или другой ноде, не даёт сохранить подключение. Цель на этой ноде отвечает своим сертификатом, и клиенты с таким именем не подключаются; так 12.10 перестал работать VLESS на двух нодах.
- Соблюдаются NOTICE и TRADEMARK Mikan: панель указывает авторство Mikan (Copyright (C) 2026 Miroshka and Mikan), говорит, что Kinkan — изменённая версия, и больше не показывает имя mikan. NOTICE.md и TRADEMARK.md — файлы Mikan, NOTICE-KINKAN.md — форка.

## 0.5.0.5-tt.12
### en
- Node site, answering like Caddy 2.11: another Host (or none, as a bare HTTP/2 preface has) gets an empty 200, CONNECT to the site a 308 to /, errors the site's own 404.html with the file's headers, and the ETag is Caddy 2.11's. The site's names are the node's certificate's.
- REALITY aimed at a web server on the node's own address (a Caddy beside the node) moves to the node's site when the node's certificate is publicly trusted for every name REALITY shows, so the address shows one web server and the Caddy can go.
- Scanner journal: each rejected REALITY client is recorded with the JA4 fingerprint of its TLS ClientHello; the journal says what kind of program it looks like and which fingerprints many addresses share (one program behind many IPs is a scan through a proxy network). The spike warning waits for three whole days of collection, so a node just moved to Kinkan no longer shows its usual background as a spike.
- Protocol check: each TCP protocol is also sent a random wrong key and compared with the node's site (VLESS, Trojan, AnyTLS, VMess, and the QUIC protocols).
- A subscription used from many networks or countries at once is flagged on the user's card (network and country only, no addresses, 30 days).
- Quiet node by default: new installs and new nodes no longer get TUIC, whose own close codes give a node away to one probe; its card says so. The TrustTunnel card describes Kinkan's answer, not Mikan's 407.
- The addon catalog (Mikan's marketplace) is checked with Mikan's key again; it was refused. A remote node still on Mikan's installer cannot follow the panel to Kinkan: its card now says why and shows the one command to run on it.
### ru
- Сайт ноды отвечает как Caddy 2.11: на чужой Host (или без него, как у «голой» преамбулы HTTP/2) пустой 200, на CONNECT к сайту 308 на /, ошибки страницей 404.html сайта с заголовками файла, ETag в формате Caddy 2.11. Имена сайта берутся из сертификата ноды.
- REALITY, который смотрит на веб-сервер на собственном адресе ноды (Caddy рядом с нодой), переходит на сайт ноды, если сертификат ноды публично доверенный для всех имён REALITY: на адресе один веб-сервер, Caddy можно убрать.
- Журнал сканеров: для каждого отклонённого REALITY-клиента записывается отпечаток JA4 его TLS ClientHello; журнал подсказывает, на какую программу он похож, и показывает отпечатки, общие для многих адресов (одна программа за многими IP означает скан через сеть прокси). Предупреждение о всплеске ждёт трёх полных дней сбора: у ноды, только что переведённой на Kinkan, обычный фон больше не выглядит всплеском.
- Проверка протоколов: каждому TCP-протоколу отправляется случайный неверный ключ, ответ сравнивается с сайтом ноды (VLESS, Trojan, AnyTLS, VMess и QUIC-протоколы).
- Подписка, которой пользуются из многих сетей или стран одновременно, помечается в карточке пользователя (только сеть и страна, без адресов, 30 дней).
- Тихая нода по умолчанию: новые установки и новые ноды больше не получают TUIC, чьи коды закрытия выдают ноду одной пробой; это написано в его карточке. Карточка TrustTunnel описывает ответ Kinkan, а не 407 Mikan.
- Каталог аддонов (маркетплейс Mikan) снова проверяется ключом Mikan, раньше он отвергался. Удалённая нода на установщике Mikan не может перейти за панелью на Kinkan: её карточка объясняет почему и показывает одну команду для её сервера.

## 0.5.0.5-tt.11
### en
- Rollback from the panel: Settings → Updates can go back to the version that ran before the last update, keeping today's data, when that update did not change the database schema (Mikan's or Kinkan's). Automatic updates are turned off first, and the server backs up twice and goes back to the current version if the previous one does not start. Only an admin session can ask for it; the image comes from the server's own record, never from the request. Remote nodes are not rolled back. See ROLLBACK.md.
- Quiet node: a UDP inbound (Hysteria2, TUIC) on 443 is no longer taken for the HTTPS site on TCP 443 as its own reference.
- Fork upkeep: syncing with Mikan follows it to its own core, Citrus, and a Mikan beta that would conflict with the fork is reported early.
### ru
- Откат из панели: в Настройках → Обновления можно вернуться к версии, которая работала до последнего обновления, с сегодняшними данными, если обновление не меняло схему базы (Mikan и Kinkan). Сначала выключается автообновление; сервер делает две резервные копии и возвращает текущую версию, если прежняя не запустилась. Попросить откат может только сессия админа; образ сервер берёт из своей записи, а не из запроса. Удалённые ноды не откатываются. Подробно в ROLLBACK.md.
- Тихая нода: подключение по UDP (Hysteria2, TUIC) на 443 больше не принимается за HTTPS-сайт на TCP 443 как за собственный эталон.
- Обслуживание форка: синхронизация с Mikan следует за ним на его ядро Citrus, а о бете Mikan, которая конфликтовала бы с форком, становится известно заранее.

## 0.5.0.5-tt.10
### en
- Quiet node: advice now follows what the check saw. TUIC's close code, Hysteria2's proxy answer and "obfs keeps it silent" are named only when their findings are there; a check that failed for another reason (an expired certificate, say) names that reason. A REALITY target in another network than the node is flagged (both networks from the panel's DB-IP database). An inbound is no longer compared with itself as its own reference, and on 443 the check does not suggest 443 as the reference.
- Check history: each protocol check is kept 30 days (at most 100 per node) and can be opened again, deleted, or cleared with the node's whole history.
- The scanner journal can be cleared: the node starts an empty journal, then the panel deletes its copy, and a collection in flight cannot bring the old rows back. The page shows when the journal was last collected, why collecting fails, how many events the node dropped, and a node that keeps no journal. One source can no longer fill the journal: made-up methods count as OTHER, a source gets at most 32 rows a day and a day at most 2,000. The panel's own checks are recognised behind Cloudflare or NAT too (by the address the node sees it come from), and Hysteria2 on a port range is recorded again.
- TrustTunnel fallback: refuses the cloud metadata services outside link-local (AWS IPv6, Alibaba); passwords are compared as SHA-256 digests, so timing tells neither a password's length nor which users exist.
- Node site: while a new site is on its way, TrustTunnel and REALITY keep pointing at the one the node still serves instead of answering 407; a site port another program held is taken again once it is free; a page title in another charset or cut inside a character no longer stops the upload.
- Installer: after a fork database migration, a failed update no longer goes back to an older Kinkan that would refuse the newer schema; a release (-tt.N) now counts as newer than its own candidates (-tt.N-rc.M).
### ru
- Тихая нода: советы опираются на то, что увидела проверка. Код закрытия TUIC, прокси-ответ Hysteria2 и «obfs молчит» называются, только если есть их находки; проверка, упавшая по другой причине (например, просроченный сертификат), называет эту причину. Цель REALITY в другой сети, чем нода, помечается (обе сети по базе DB-IP панели). Подключение больше не сравнивается само с собой как с эталоном, а на 443 проверка не предлагает эталон 443.
- История проверок: каждая проверка протоколов хранится 30 дней (не больше 100 на ноду); её можно открыть снова, удалить или очистить всю историю ноды.
- Журнал сканеров можно очистить: нода начинает пустой журнал, затем панель удаляет свою копию, и идущий сбор не вернёт старые записи. Страница показывает, когда журнал собирался, почему сбор не удаётся, сколько событий нода потеряла и ноду без журнала. Один источник больше не забивает журнал: выдуманные методы считаются как OTHER, на источник не больше 32 строк в день, на день не больше 2000. Свои проверки панели узнаются и за Cloudflare или NAT (по адресу, с которого её видит нода), а Hysteria2 на диапазоне портов снова пишется в журнал.
- Fallback TrustTunnel: не пускает к сервисам метаданных облаков вне link-local (IPv6 AWS, Alibaba); пароли сравниваются как SHA-256, и по времени ответа не узнать ни длину пароля, ни какие пользователи есть.
- Сайт ноды: пока новый сайт в пути, TrustTunnel и REALITY смотрят на тот, который нода ещё отдаёт, а не отвечают 407; порт сайта, занятый другой программой, подхватывается, когда освободится; заголовок страницы в другой кодировке или обрезанный посреди символа больше не мешает загрузке.
- Установщик: после миграции базы форка неудачное обновление больше не откатывается на старый Kinkan, который отказался бы от новой схемы; релиз (-tt.N) теперь новее своих кандидатов (-tt.N-rc.M).

## 0.5.0.5-tt.9
### en
- Quiet node: after the protocol check the panel says what the node looks like as a whole (no quieter than its loudest inbound: all protocols share its address) and, for each inbound that is not quiet, why and what helps: turn TUIC or another proxy answer off, give the node a site (TrustTunnel without a fallback, REALITY on the panel's empty 404), turn on Hysteria2's obfs, or check again with a reference on 443. Turning an inbound off asks first and is undone under Protocols.
### ru
- Тихая нода: после проверки протоколов панель показывает, как выглядит нода целиком (не тише самого заметного подключения: адрес у всех протоколов общий), и для каждого не тихого подключения — почему и что поможет: выключить TUIC или другой прокси-ответ, дать ноде сайт (TrustTunnel без fallback, REALITY на пустом 404 панели), включить obfs у Hysteria2 или повторить проверку с эталоном на 443. Выключение спрашивает подтверждение и отменяется в «Протоколах».

## 0.5.0.5-tt.8
### en
- Scanner journal: the panel's own checks of its nodes are marked as such, hidden by default and kept out of the chart and the spike; only the panel can mark them. Country and network come from DB-IP's free databases (CC BY 4.0), which the panel keeps and updates monthly, where the node found none.
- The fork's code sits beside Mikan's and is called with one line each, so syncs with Mikan merge cleanly: a trial merge of Mikan 0.5.0.5-rc.4 went from 15 conflicting files to 3. The Node API is unchanged; the node's served site moved to its own endpoint.
- A server on a Mikan newer than Kinkan's latest release is told that it stays on Mikan until a Kinkan release made on that Mikan is out, instead of "is the latest release".
- The kumquat logo in the panel, the favicon and the touch icon; Kinkan's own README.
### ru
- Журнал сканеров: собственные проверки панели помечаются, по умолчанию скрыты и не входят в график и всплеск; пометку ставит только панель. Страну и сеть, если нода их не определила, подставляет панель по бесплатным базам DB-IP (CC BY 4.0), которые она хранит и обновляет раз в месяц.
- Код форка лежит рядом с кодом Mikan и подключается одной строкой, поэтому синхронизация с Mikan сливается без конфликтов: пробное слияние с Mikan 0.5.0.5-rc.4 — 3 файла с конфликтами вместо 15. Node API не изменился; то, какой сайт отдаёт нода, теперь отдельный запрос.
- Сервер на Mikan новее последнего релиза Kinkan получает объяснение, что остаётся на Mikan до релиза Kinkan на этой версии, а не «is the latest release».
- Логотип-кумкват в панели, favicon и иконке для телефона; собственный README Kinkan.

## 0.5.0.5-tt.7
### en
- The protocol check says "Not enough data" instead of "Not checked" when its probes ran but had nothing to compare with, and explains why next to the verdict. A clear TUIC giveaway still reads "Exposed" even if other probes failed.
- An optional cover hostname for the check: the inbound and the reference site may use different TLS names on the same pinned node IP. Without it the check picks the name as before.
### ru
- Проверка протоколов пишет «Недостаточно данных» вместо «Не проверено», когда пробы прошли, но сравнить было не с чем, и объясняет причину рядом с вердиктом. Явный признак TUIC по-прежнему даёт «Палится», даже если другие пробы не удались.
- Необязательное имя сайта-прикрытия для проверки: подключение и эталонный сайт могут иметь разные TLS-имена на одном закреплённом IP ноды. Без него имя выбирается как раньше.

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
