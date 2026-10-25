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

Клиент добавляет только свой service token и `reqctx.RequestID`; чужая outgoing metadata, пользовательский authorization и hints не пересылаются. Unary без deadline получает 5 секунд; существующий deadline сохраняется. Streaming требует собственного deadline и не получает unary default. Retry до трёх попыток только UNAVAILABLE для явно перечисленных безопасных методов; idempotency-key retry остаётся у приложения. `FileToken` перечитывает изменённый файл для ротации projected tokens. DialOptions позволяют service-owned TLS credentials/dialer; исходный default — plaintext HTTP/2 внутри защищённой сети кластера.

Server устанавливает request ID, проверяет service identity → policy → user identity → tombstone → protobuf validation до handler, восстанавливает panic, нормализует статусы и сохраняет finerr details. Неизвестный метод запрещён; Serve отказывает при неполной policy. Unary и streaming проходят ту же auth-цепочку; каждое входящее stream-сообщение валидируется. В `reqctx` один набор ключей для server/client/handlers; setters принимают только доверенные значения от сборки сервиса.

`Rule.AllowDeleted` перенесён из действующего user: false по умолчанию; true только для exact ModelUser cleanup methods; wildcard и другие модели отклоняются Check/Serve. Не обходит service caller, валидность токена, subject или tombstone lookup failure. Не задаётся metadata/request. Это сохраняет реальную семантику user при будущем подключении библиотеки. `ModelUser`, `ModelSystem`, `ModelMixed`, `ModelNoUser`, wildcard policy и exact-method precedence сохранены.

`DevServiceVerifier`, `DevUserVerifier`, `DevToken` и Reflection допустимы только при явной dev-сборке сервиса. Библиотека не читает окружение и сама не включает dev-режим. Логгер/registry/verifiers передаются сервисом. Debug-логи маскируют token-поля и bytes protobuf на любой глубине, secret metadata и binary metadata. Logger enrichment значениями reqctx остаётся у сервиса.

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
