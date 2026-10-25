# Проверки библиотеки и покрытие FIN-243

## Уровни

- Unit: изолированные функции и ветви без БД и сети, на фейках объявленных интерфейсов.
- Integration: настоящие PostgreSQL18, миграции/транзакции/блокировки, transport и фоновые процессы там, где они есть.
- API: публичный Go API со стороны потребителя; для stateful контрактов реальный PostgreSQL и транзакция вызывающего. Прямой вызов private реализации не становится API из-за имени теста. Примеры с Output исполняются в API, без Output компилируются, как раньше.

TestUnit/TestIntegration/TestAPI и FuzzUnit/FuzzIntegration/FuzzAPI задают селекторы. Race inventory блокирует неклассифицированные Test/Fuzz до каждого сборщика; отдельный регрессионный тест запускает настоящий Go-модуль с Example, fuzz seed и race-only тестом. Собственного реестра тестов нет.

## Команды и метрика

`make collect-{unit,integration,api}` собирает отдельные JSONL и профили Go statements с race/count1/timeout. `make check-{unit,integration,api}` сначала проверяет execution evidence и профиль, затем запускает go-test-coverage/v2 v2.20.1 с total80. Принимается штатное округление79,95→80,0. `make cover` требует все три порога. `make cover-combined` объединяет исходные профили штатным comma-входом; результат информационный.

Для всех трёх профилей инструментируется одинаковый список `go list -race ./...` за исключением только точного пакета `<module>/internal/pgtest` (решение владельца 25.10.2026). Тесты выполняются по всему `./...`, включая pgtest. Generated production code и остальные обычные `.go` helpers учитываются, одноимённые соседние и вложенные пакеты не исключаются. Сравнивать старые уникальные строки FIN-210 с новой statements-метрикой нельзя. Проценты/merge рассчитывает только готовый инструмент.

Guard требует реально выполненный passing Test/Example/Fuzz event, отсутствие skipped/failed тестов и исполняемый statement block. Package skip при отсутствии тестов допустим; Test skip — нет. Пустой файл, отсутствующий, повреждённый и zero-statement профиль не даёт зелёный статус; исключение для точного header-only профиля исходного шаблона описано ниже. PostgreSQL без Docker/TEST_DATABASE_URL при CI=true — ошибка. Каждый агентский запуск также ограничивается process timeout.

CI сохраняет unit/integration/API/combined reports, native breakdowns, JSONL и профили как artifacts. Его прежний required check `go` проверяет все шесть collector/check outcomes; промежуточный continue-on-error не скрывает ошибку. Права только read, комментариев coverage нет, fork PR сохраняет ограничения. Job `api` по-прежнему означает gorelease compatibility и не заменяет API-тесты.

`make test`, `make lint`, `make api` выполняются до публикации. Release/CHANGELOG/migrations и определения Renovate checks сохранены. В FIN-245 controller временно приостанавливался, затем по отдельной команде владельца вновь включён. FIN-247 не меняет флаг, защиты или quarantine: валидный N/A устраняет причину постоянно красного coverage только в пустом библиотечном шаблоне. Реальное падение любого required check по-прежнему блокирует merge; controller действует по прежним правилам retry/close. См. docs/renovate-policy.md. Новые версии и теги не создаются.

## FIN-250

Unit используют fake invoker/stream/verifiers и чистые context/policy/status операции без сети. Integration проверяют реальные grpc.Server/client transport, retry/deadline, stream, validation/masking и остановку. API выполняются внешними пакетами `_test` через публичные server/client/reqctx, generated clients и фактическую цепочку. Private helpers не выдаются за API. SQL и бизнес-RPC не добавляются.

Эта новая библиотека не получает template N/A: фиксированная identity `fin-assist/go-library-template` в check-suite.sh сохранена намеренно. Все три suites обязаны иметь production statements и total80. Runtime не использует PG, internal/pgtest оставлен как тестовая инфраструктура; только этот точный пакет исключён из одинакового instrumentation list.
