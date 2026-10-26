# Изменения

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии и выпуск — [docs/releasing.md](docs/releasing.md). Каждый PR библиотеки, включая документацию, тесты и CI, добавляет строку в «Unreleased»; исключение — полностью ботские PR Renovate только с зависимостями, их сводку по `go.mod` добавляет PR выпуска. При выпуске раздел получает номер версии и дату.

## [Unreleased]

### Несовместимо

- FIN-299: `ModelMixed` больше не превращает вызов без проверенного токена пользователя в системный: заголовок `authorization`, который не является `Bearer <token>` (в том числе пустое значение или пустое первое из нескольких), и вызов `bank-bff` без токена пользователя получают `UNAUTHENTICATED` `INVALID_TOKEN` до handler (раньше handler запускался как системный вызов с `user_id` из запроса). `ServiceVerifier`, вернувший пустое имя без ошибки, даёт голый `UNAUTHENTICATED`; `Check`/`Serve` отклоняют пустое имя в `Rule.Callers`. Сервисам правки не нужны: ни одна текущая `AccessPolicy` не использует `ModelMixed` и пустые имена; системный вариант смешанного метода остаётся у остальных вызывающих без `authorization`.

### Изменено

- FIN-341: правила lint о процессном состоянии приведены к ядру go-library-template (FIN-322): секции `forbidigo` и `depguard` в `.golangci.yml` и `scripts/test_lint_policy.py` совпадают с шаблоном. Добавлены запреты `slog.With` и `os.Exit`; сняты правила вне ядра: `grpclog` и `grpclog/glogger`, `prometheus.Unregister`, `http.DefaultTransport`/`DefaultServeMux`/`PostForm`, `otel.Tracer`/`Meter`/`Handle` и `otel/log/global`, `os.UserHomeDir`/`UserConfigDir`/`UserCacheDir`, `os.TempDir`/`CreateTemp`/`MkdirTemp`, `(*os.File).Chdir`, `runtime.SetDefaultGOMAXPROCS`, `syscall`/`golang.org/x/sys`; принцип в целом проверяется на ревью. Runtime и экспортируемый API не менялись.
- FIN-337 (по FIN-308, FIN-309): из go-library-template перенесены запреты `forbidigo` и `depguard` FIN-294 (стандартные логгеры, глобальный реестр метрик, `http.DefaultClient` и функции пакета `http`, глобальные провайдеры `otel`, `os/signal`, окружение через `os`/`syscall`/`golang.org/x/sys`, функции `flag`, `os.Stdout`/`Stderr`/`Stdin`, `os.TempDir`/`CreateTemp`/`MkdirTemp`, `runtime/debug.Set*`, `runtime.GOMAXPROCS`, смена рабочего каталога; пустой импорт `expvar`, `net/http/pprof`, `golang.org/x/net/trace`, `grpclog/glogger`, `glog`, `klog`, `automaxprocs`, `automemlimit`) с проверкой `scripts/test_lint_policy.py` (версии prometheus, grpc, otel и x/sys в пробном модуле берутся из `go.mod`, вручную закреплены только otel/log, klog, automaxprocs, automemlimit); защита CI FIN-301: checkout в jobs `lint`, `api`, `migrations` без сохранённого токена, `base_ref` передаётся в скрипт через окружение, проверка `scripts/test_workflow_hardening.py`; формулировки FIN-307 в `docs/releasing.md` и вступлении CHANGELOG: строка в `[Unreleased]` от каждого PR, включая документацию, тесты и CI, заголовок PR выпуска с ключом FIN. Нарушений новых запретов в коде библиотеки нет; runtime и экспортируемый API не менялись.
- FIN-321: `internal/pgtest` больше не копирует go-migrate (по FIN-310): оставлен только выбор тестового PostgreSQL (`TEST_DATABASE_URL`/testcontainers, skip локально, ошибка в CI), создание БД, правила файлов и обход up → down → up делегированы `github.com/fin-assist/go-migrate` v0.1.1. go-migrate импортируется только тестами, но как require модуля входит в граф модулей и повышает минимальную версию go-migrate у сервисов, импортирующих библиотеку (сервисы уже на v0.1.1). Исключение `internal/pgtest` из покрытия не изменилось; runtime и экспортируемый API не менялись.
- FIN-316: из `AGENTS.md` убрано временное правило запрашивать ревью только у Codex: квота Claude восстановлена, ревью снова запрашивается одним комментарием `@codex @claude` по общему правилу Workflow. Runtime и API библиотеки не менялись.

### Исправлено

- FIN-264: проверка версии в CI сохраняет сравнение с предыдущим выпуском при публикации тега во время gorelease; чужие и изменившиеся теги отклоняются. Runtime и публичный API не меняются.

- FIN-300: debug-журнал вызова больше не пишет отвергнутые значения `x-request-id` (неверная форма, длиннее 128 символов) — каждое такое значение заменяется на `(rejected)`, корректные логируются как раньше. Добавлены транспортные unit/integration/API тесты `x-request-id`, валидации каждого сообщения client stream и `Shutdown`, дожидающегося завершающегося потока; уточнены doc comment `Options.MaxRecvMsgSize` (16 МиБ `SearchOperations` — ответ, а не приём) и README о дедлайне потоков.

## [v0.1.0] — 2026-10-25

### Добавлено

- FIN-254: подготовлен первый выпуск go-grpc v0.1.0 с накопленными изменениями FIN-250 и FIN-260; публикация версии и установка тега выполняются владельцем отдельно.

- FIN-250: публичные server/client/reqctx из шаблона сервиса: порядок identity/policy/tombstone/validation/recovery, masking, RED, deadlines/retry/stream и shutdown; сохранена cleanup policy AllowDeleted из user. Реальные transport и внешние consumer API тесты, независимые total80 и строгие guards. Библиотека не содержит миграций и не меняет сервисы; ключи и tombstone storage остаются в существующих библиотеках.

### Исправлено

- FIN-250: устранены подтверждённые ревью утечки raw error/SQL/DSN/token, panic payload и stack trace в общем gRPC-журнале, включая ошибки verifier/tombstone/validation. Логи содержат безопасные event/method/code/reason; RPC status/details, RED и recovery сохранены. Негативные unit/integration/API проверки unary и streaming больше не закрепляют небезопасный исходный logging.

- FIN-260: выключенный Renovate controller завершается до credential/config/event/API; непривилегированный workflow gate допускает Environment только при точном case-sensitive true. Явный bootstrap отдельно требует credential, enabled guards сохранены; добавлены негативные CLI/workflow регрессии. Runtime библиотеки и независимое покрытие не меняются.
