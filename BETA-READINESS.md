# Kinkan: подготовка беты

База: tt.13 (21043a1), слита подготовленная ветка merge/mikan-0.5.1.0.
Перенос на Citrus и новые функции Mikan 0.5.1.1 выполняются после проверки этой базы.
Документ фиксирует результаты 17.10.2026; не является объявлением готового релиза.

## Проверено локально

- Слияние без конфликтов; сборка cmd/mikan и cmd/mikan-node.
- pnpm install --frozen-lockfile и production build: TypeScript, contrast, Vite.
- go vet ./....
- Тесты proto, release, ttprobe, nodeprobe, scannerlog, panel/updates, mikan-release.
- Race-тесты nodeapi, nodeprobe, ttprobe, scannerlog, panel/updates.
- Пакет node на Windows не зелёный: TestNodeSaysWhoHoldsPort80 (код ошибки bind),
  TestPutSiteChecksTheArchive (Unix-права 0600). Нужен запуск на Linux; проверки не отключались.
- Cargo.lock обновлён с rustls 0.23.45 до 0.23.46 точно по upstream 804960ebf.
  Cargo audit/test/build ещё не выполнены: локальный Rust toolchain отсутствует.

## Условия беты — пока не закрыты

- [ ] Linux CI целиком: PostgreSQL 18, go test -race ./..., vet, govulncheck.
- [ ] Rust: fmt, clippy, test, locked build, cargo audit, fault-injection установщика.
- [ ] Проверить воспроизводимость обеих third_party-копий по патчам.
- [ ] Проверить OpenAPI/schema после слияния, не только TypeScript build.
- [ ] Изолированный Linux-стенд: чистая установка, регистрация ноды, сертификаты/ACME.
- [ ] Обновление с опубликованного tt.13: данные и подписки сохраняются.
- [ ] Откат и восстановление из бэкапа с учётом миграций БД 0.5.1.0.
- [ ] Authenticated TCP/UDP по поддерживаемым протоколам, fallback и scanner probes.
- [ ] Мобильный клиент: холодный старт, тишина, обрыв, смена сети.
- [ ] Перед выпуском переписать устаревшие статусы ROADMAP/RELEASE-TT по результатам.

## Следующий этап: Citrus

Закрепить конкретный тег/коммит; составить таблицу собственных патчей и upstream-аналогов.
Принятые TT fallback/auth-event/client fixes повторно не накладывать.
Проверить заголовки и SETTINGS, сайт ноды, trusted proxies, сканерные события,
учёт/лимиты пользователей, перезагрузку listener и соответствие подписок клиентам.
Миграцию проверять отдельно от новых функций диагностики и DNS Mikan.

Продакшен не обновлялся. Бета не опубликована.
