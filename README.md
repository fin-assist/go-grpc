# go-grpc

Общая gRPC-инфраструктура «Понятных финансов» ([FIN-250](https://finassist.youtrack.cloud/issue/FIN-250), эпик FIN-248): `server`, `client`, `reqctx` в одном модуле `github.com/fin-assist/go-grpc`. Извлечена из go-service-template; бизнес-handlers, `wire()`, static access policy и lifecycle остаются в сервисах. Сервисы этим PR не изменяются.

## Внедрение

```go
srv, err := server.New(server.Options{}, server.Deps{
    Logger: logger, Metrics: registry, Policy: servicePolicy,
    Services: serviceVerifier, Users: userVerifier, Tombstones: tombstones,
})
// Handle err; register generated handlers on srv.GRPC() before Serve.
err = srv.Serve(listener)
// On shutdown: srv.Shutdown(shutdownContext, balancerDrainDelay).
```

`ServiceVerifier`, `UserVerifier`, `Tombstones` — узкие интерфейсы потребителя: реализации предоставляет go-identity/go-tombstone. Ключи и storage не дублируются. `server.NoTombstones` разрешён только сервисам без пользовательских данных. Все зависимости обязательны. Реестр метрик отдельный для экземпляра; повторная регистрация — ошибка. Сервис владеет listener и shutdown context; `SetServing` следует readiness сервиса.

```go
metrics, err := client.NewMetrics(registry)
// Handle err.
conn, err := client.Dial(target, client.Options{
    Peer: "analytics", Tokens: &client.FileToken{Path: serviceTokenFile},
    Retry: []string{"finassist.analytics.v1.AnalyticsService/GetAnalytics"},
    Metrics: metrics,
})
// Handle err; use the generated client and close conn during service shutdown.
```

Клиент добавляет только свой service token и `reqctx.RequestID`; чужая outgoing metadata, пользовательский authorization и hints не пересылаются. Unary без deadline получает 5 секунд; существующий deadline сохраняется. Streaming не получает unary default: deadline потока задаёт вызывающий из бюджета grpc-api.md 9, клиент его не добавляет и не требует. Retry до трёх попыток только UNAVAILABLE для явно перечисленных безопасных методов; idempotency-key retry остаётся у приложения. `FileToken` перечитывает изменённый файл для ротации projected tokens. DialOptions позволяют service-owned TLS credentials/dialer; исходный default — plaintext HTTP/2 внутри защищённой сети кластера.

Server сохраняет корректный `x-request-id` (`[A-Za-z0-9._-]`, до 128 символов) без изменений, иначе заменяет новым (берётся только первое значение ключа); каждое отвергнутое значение в debug-журнале metadata заменено на `(rejected)`. Server проверяет service identity → policy → user identity → tombstone → protobuf validation до handler, восстанавливает panic, нормализует статусы и сохраняет finerr details. Неизвестный метод запрещён; Serve отказывает при неполной policy. `ModelMixed` выбирает пользовательский вариант по наличию ключа `authorization`: неразбираемое или пустое значение и вызов `bank-bff` без токена пользователя — `INVALID_TOKEN`, а не системный вызов (FIN-299, grpc-api.md 3.1, 3.3). Верификатор, вернувший пустое имя вызывающего, даёт голый `UNAUTHENTICATED`; пустое имя в `Rule.Callers` отклоняется Check/Serve. Unary и streaming проходят ту же auth-цепочку; каждое входящее stream-сообщение валидируется. В `reqctx` один набор ключей для server/client/handlers; setters принимают только доверенные значения от сборки сервиса.

`Rule.AllowDeleted` перенесён из действующего user: false по умолчанию; true только для exact ModelUser cleanup methods; wildcard и другие модели отклоняются Check/Serve. Не обходит service caller, валидность токена, subject или tombstone lookup failure. Не задаётся metadata/request. Это сохраняет реальную семантику user при будущем подключении библиотеки. `ModelUser`, `ModelSystem`, `ModelMixed`, `ModelNoUser`, wildcard policy и exact-method precedence сохранены.

`DevServiceVerifier`, `DevUserVerifier`, `DevToken` и Reflection допустимы только при явной dev-сборке сервиса. Библиотека не читает окружение и сама не включает dev-режим. Логгер/registry/verifiers передаются сервисом. Debug-логи маскируют token-поля и bytes protobuf на любой глубине, secret metadata и binary metadata. Logger enrichment значениями reqctx остаётся у сервиса. Security correction относительно исходного шаблона: raw handler/verifier/tombstone/validation errors, panic payload и stack trace больше не пишутся в журнал; остаются безопасные event/method/code/reason и прежние masked metadata/request. RPC status/details, metrics и recovery сохраняются.

## Метрики

Сохранены совместимые имена и bounded labels:

| Имя | Labels |
|---|---|
| `grpc_server_handled_total` | method, code, reason |
| `grpc_server_handling_seconds` | method |
| `finassist_grpc_denied_total` | method, caller, cause |
| `grpc_client_handled_total` | peer, method, code |
| `grpc_client_handling_seconds` | peer, method |

Peer/method/policy — статические значения сервиса; user ID и request ID не labels. Client RED относится к завершённым unary calls; streaming сохраняет прежнюю семантику без новых RED series.

## Проверки и выпуск

`make test`, `make lint`, `make cover`, `make api`. Локальные процессы ограничиваются timeout. Unit без БД/сети; integration — настоящий gRPC transport; API — внешний Go-потребитель server/client/reqctx, generated clients и фактическая цепочка. PostgreSQL не нужен runtime этой библиотеки; сохранённый test harness использует TEST_DATABASE_URL или Docker. Миграций библиотеки нет.

[docs/testing.md](docs/testing.md): независимые total80 unit/integration/API, закреплённый go-test-coverage/v2 v2.20.1, accepted79,95% rounding. Один `go list -race ./...` denominator, исключён только exact `<module>/internal/pgtest`; suite остаётся `./...`. Пустые/skipped/failed suites и неисполняемый профиль отклоняются. N/A исходного шаблона сюда не переносится. Combined — информационный штатный union. Required job `go` проверяет все шесть collection/check outcomes; artifacts содержат профили, JSONL, отчёты и breakdowns. Pipeline Python regressions сохранены.

`api` — gorelease, `lint` — golangci-lint, `migrations` защищает released SQL, release/Renovate checks сохранены. [CHANGELOG.md](CHANGELOG.md), [docs/releasing.md](docs/releasing.md), [AGENTS.md](AGENTS.md). Первый тег ставит владелец отдельно; merge/tags этим PR не выполняются. Подключение сервисов — отдельные задачи.

## Выключенный контроллер FIN-260

`RENOVATE_POLICY_ENABLED` включает controller только при точном строчном `true`. Отсутствие/false/TRUE/True — успешный no-op до token/config/event/API. Отдельный непривилегированный workflow gate не получает secrets, Environment и checkout; только его канонический output допускает reconcile в main-only Environment. Явный `--bootstrap` остаётся отдельной credential-required операцией и не включает controller. Enabled guards, проверки защиты и текущего head сохранены. Этот PR не меняет флаги/защиты/policy branch; bootstrap/readback координируется отдельно по FIN-260.

Проверка `api` устойчива к публикации тега во время CI: release-коммит сравнивается
с предыдущим выпуском, удалённый тег проверяется по commit до и после `gorelease`.
Подробности и границы исключения — [docs/releasing.md](docs/releasing.md#проверка-версии-в-ci-fin-264).

## Общий Go CI и линтеры (FIN-372)

`.finassist-ci.json` закрепляет общий источник `fin-assist/go-ci` полным SHA и объявляет профиль/исключения. CI вызывает reusable workflows того же SHA; `make lint`, `make fmt`, `make lint-config` и `make lint-migrations BASE_REF=origin/main` используют его через `scripts/common_ci.py`. Конфигурации создаются в игнорируемом `.finassist-ci/generated/`; локальные копии правил не редактируются. Первый локальный вызов требует доступ к GitHub. Обновление workflow и manifest pin приходит одним PR Renovate; откат возвращает прежние SHA вместе. [Источник, профили, обновление и откат](https://github.com/fin-assist/go-ci/blob/main/docs/shared-go-ci.md). Проверки тестов/coverage, PostgreSQL 18, race и совместимость сохраняются.

28.10.2026: контроллер FIN-211 в этом репозитории намеренно выключен. Правила обновления общего CI находятся непосредственно в `renovate.json`, чтобы SHA workflows, входные `revision` и manifest обновлялись одной группой без синхронизации ветки `renovate-policy`. Её presets, подписанные metadata и запрет встроенного automerge сохраняются; контроллер не активируется.

Общий CI использует `GO_MODULES_TOKEN` только на `push`; PR, включая ветки того же репозитория, не получают секретов. Текущие зависимости публичны. Для PR с приватными зависимостями одного добавления секрета недостаточно: нужен отдельно согласованный доверенный pipeline.

Renovate обновляет SHA workflows, входные `revision` и manifest. Если в новом источнике изменился `common/bootstrap.py`, в том же PR синхронизируйте `scripts/common_ci.py` с этим закреплённым файлом: проверка побайтного равенства до этого оставляет CI красным. Конфиги продолжают храниться в общем источнике. Откат возвращает согласованные pins и соответствующую копию bootstrap. Ручной коммит bootstrap делает PR ручным: автомерж FIN-211 для полностью ботских обновлений к нему не применяется.
