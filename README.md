# Room Booking Platform

Микросервисная система бронирования переговорных комнат на Go 1.25 и PostgreSQL. Проект показывает разделение bounded contexts, конкурентный доступ к слотам, координацию распределённой операции через компенсацию и надёжную обработку фоновых задач.

## Архитектура

```text
                         ┌───────────────────────┐
                         │ API Gateway :8080     │
                         │ JWT / RBAC / proxy    │
                         └──────┬────────────────┘
             ┌──────────────────┼────────────────────┐
             │                  │                    │
      ┌──────▼─────┐     ┌──────▼─────┐      ┌──────▼──────────┐
      │ Auth       │     │ Room       │      │ Availability    │
      │ :8081      │     │ :8082      │      │ :8083           │
      │ auth_db    │     │ room_db    │      │ availability_db │
      └────────────┘     └────────────┘      └────────┬────────┘
                                                      │ reserve/release
                                              ┌───────▼────────┐
                                              │ Booking :8084  │
                                              │ booking_db     │
                                              └───────┬────────┘
                                                      │ retry worker
                                              ┌───────▼────────┐
                                              │ Conference mock│
                                              │ :8085          │
                                              └────────────────┘
```

Каждый доменный сервис владеет своей базой. Внешний трафик проходит через gateway; внутренние endpoints доступны только в сети Docker Compose.

## Ключевые инженерные решения

### Конкурентное бронирование

- Слот блокируется через `SELECT ... FOR UPDATE` в транзакции.
- Переход `free → booked` выполняется только после проверки времени и текущего статуса.
- `UNIQUE(room_id, start_at)` и PostgreSQL exclusion constraint запрещают дубли и пересечения.
- Частичный индекс по свободным слотам ускоряет основной read path.

### Согласованность между сервисами

Создание бронирования — saga с компенсацией:

1. availability-service атомарно резервирует слот с заранее созданным `booking_id`;
2. booking-service сохраняет бронирование;
3. при ошибке локальной записи выполняется идемпотентное освобождение слота.

Бронирование и `conference_job` записываются **в одной PostgreSQL-транзакции**, поэтому состояние `conference_status=pending` не может появиться без фоновой задачи.

### Надёжный conference worker

- Один SQL CTE атомарно выбирает и переводит задачи в `processing`.
- `FOR UPDATE SKIP LOCKED` позволяет безопасно запускать несколько worker-реплик.
- Lease возвращает зависшие после падения задачи в обработку.
- Ошибки внешнего сервиса повторяются с exponential backoff, максимум пять попыток.
- Успех/окончательная ошибка обновляют booking и job одной транзакцией.
- Семантика доставки — at-least-once; внешний mock детерминирован и идемпотентен по `booking_id`.

### Безопасность

- JWT подписывается RS256; валидатор принимает только RS256 и проверяет issuer, срок и authorization claims.
- Приватных ключей в репозитории и Docker image нет. Для локального запуска RSA-3072 keypair генерируется с правами `0600` в отдельном Docker volume.
- `/dummyLogin`, `/seed` и самостоятельная регистрация admin доступны только при `TEST_TASK_MODE=true`.
- JSON decoder отклоняет неизвестные поля и несколько документов в одном body.
- Пароли ограничены требованиями bcrypt и сохраняются только как bcrypt hash.
- HTTP-серверы и межсервисные клиенты имеют timeouts; сервисы завершаются graceful shutdown.

> `docker-compose.yml` по умолчанию оставляет `TEST_TASK_MODE` выключенным. Команда `make up` включает его только для локального демо и E2E. В deployment-конфигурации режим должен оставаться выключенным, а ключи должны поступать из secret manager.

## Быстрый старт

Требуются Docker и Docker Compose.

```bash
make up
```

Команда собирает сервисы, ожидает health checks, запускает fail-fast миграции и создаёт тестовые учётные записи. API доступен на `http://localhost:8080`.

Если `8080` занят: `GATEWAY_PORT=18080 make up`.

```bash
make down
```

`make down` удаляет локальные контейнеры и volumes, включая сгенерированный JWT keypair и данные PostgreSQL.

## Проверки

```bash
make test          # unit и component tests
make test-race     # тесты с race detector
make vet           # go vet для каждого модуля
make build         # сборка всех пакетов
make test-e2e      # при запущенном стеке
make vuln          # govulncheck
make verify        # локальный аналог quality job в CI
```

GitHub Actions выполняет форматирование, `go vet`, race tests, сборку, `govulncheck` и отдельный E2E-прогон полного Docker Compose стека.

Содержательные тесты проверяют, в частности:

- запрет test-only auth endpoints в production mode;
- запрет self-register admin;
- строгую проверку JWT algorithm/issuer/claims;
- rollback при невозможности атомарно создать booking и conference job;
- транзакционный claim задач worker'ом;
- rollback при частичном завершении conference job;
- сохранение активного бронирования, если освобождение слота не подтверждено;
- retries, обработку отмены и распространение ошибок worker'а;
- полный сценарий создания комнаты, расписания, бронирования и отмены.

## API

Полный контракт находится в [`api.yaml`](api.yaml).

| Method | Path | Access |
|---|---|---|
| `POST` | `/register` | public, создаёт только `user` вне test mode |
| `POST` | `/login` | public |
| `POST` | `/dummyLogin` | только `TEST_TASK_MODE=true` |
| `GET` | `/rooms/list` | authenticated |
| `POST` | `/rooms/create` | admin |
| `POST` | `/rooms/{roomId}/schedule/create` | admin |
| `GET` | `/rooms/{roomId}/slots/list` | authenticated |
| `POST` | `/bookings/create` | user |
| `GET` | `/bookings/my` | user |
| `GET` | `/bookings/list` | admin |
| `POST` | `/bookings/{bookingId}/cancel` | owner |

## Осознанные ограничения

- Межсервисная saga не является общей ACID-транзакцией. Компенсация идемпотентна, но для production нужны reconciliation worker и client idempotency key на создание бронирования.
- Internal endpoints защищены сетевой изоляцией Compose; в production нужны mTLS или service identity.
- Нет rate limiting, refresh tokens, password reset и аудита действий администратора.
- Время хранится в UTC; пользовательские часовые пояса не реализованы.
- Сервис конференций — fault-injection mock, а не интеграция с реальным провайдером.
