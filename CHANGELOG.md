# Изменения

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии и выпуск — [docs/releasing.md](docs/releasing.md). Каждый PR, который меняет API или поведение библиотеки, добавляет строку в «Unreleased»; при выпуске раздел получает номер версии и дату.

## [Unreleased]

### Изменено

- FIN-321: `internal/pgtest` больше не копирует go-migrate (по FIN-310): оставлен только выбор тестового PostgreSQL (`TEST_DATABASE_URL`/testcontainers, skip локально, ошибка в CI), создание БД, правила файлов и обход up → down → up делегированы `github.com/fin-assist/go-migrate` v0.1.1. go-migrate импортируется только тестами, но как require модуля входит в граф модулей и повышает минимальную версию go-migrate у сервисов, импортирующих библиотеку (сервисы уже на v0.1.1). Исключение `internal/pgtest` из покрытия не изменилось; runtime и экспортируемый API не менялись.

## [v0.1.0] — 2026-10-25

### Добавлено

- FIN-254: подготовлен первый выпуск go-grpc v0.1.0 с накопленными изменениями FIN-250 и FIN-260; публикация версии и установка тега выполняются владельцем отдельно.

- FIN-250: публичные server/client/reqctx из шаблона сервиса: порядок identity/policy/tombstone/validation/recovery, masking, RED, deadlines/retry/stream и shutdown; сохранена cleanup policy AllowDeleted из user. Реальные transport и внешние consumer API тесты, независимые total80 и строгие guards. Библиотека не содержит миграций и не меняет сервисы; ключи и tombstone storage остаются в существующих библиотеках.

### Исправлено

- FIN-250: устранены подтверждённые ревью утечки raw error/SQL/DSN/token, panic payload и stack trace в общем gRPC-журнале, включая ошибки verifier/tombstone/validation. Логи содержат безопасные event/method/code/reason; RPC status/details, RED и recovery сохранены. Негативные unit/integration/API проверки unary и streaming больше не закрепляют небезопасный исходный logging.

- FIN-260: выключенный Renovate controller завершается до credential/config/event/API; непривилегированный workflow gate допускает Environment только при точном case-sensitive true. Явный bootstrap отдельно требует credential, enabled guards сохранены; добавлены негативные CLI/workflow регрессии. Runtime библиотеки и независимое покрытие не меняются.
