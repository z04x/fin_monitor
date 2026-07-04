# MarketAnalyzer — Технический план (v2)

**Стек:** Go (модульный монолит) + Next.js (App Router, pnpm) + PostgreSQL
**Источники данных:** Finnhub (free) + Tiingo (free) — оба по всем тикерам, без вайтлиста
**Цель:** личный инструмент для торговли на отчётах (earnings) + артефакт в портфолио

v4 (текущая): добавлена Phase MVP-0 — Telegram-бот с утренним дайджестом (без БД, без сервера) как первый вертикальный срез архитектуры; зафиксирован монорепо; добавлен раздел по домену/хостингу.
v3: подтверждён исторический бэкфилл календаря (free-тариф отдаёт прошлые месяцы — проверено запросом за окт-2024); принят sqlc; структура переведена на domain/adapters/pipeline + ADR.
v2: исправлены два бага v1 (дата отчёта для расчёта реакции; уникальный ключ календаря при переносе даты), SPY-relative реакция, pgx/v5, golang-migrate, rate-limit клиент, recharts, CI, single-binary деплой.

---

## 1. Источники данных (проверено вживую)

Все эндпоинты протестированы реальным ключом на free-тарифе по произвольным тикерам (MU, STZ) — вайтлиста нет.

### Finnhub — фундамент (earnings, фундаменталка, консенсус, новости)

| Что | Эндпоинт | Отдаёт |
|---|---|---|
| Календарь отчётов | `GET /calendar/earnings?from=&to=` | **дата публикации**, `hour` (bmo/amc), quarter, year, `epsEstimate`, `revenueEstimate`, `epsActual`, `revenueActual` |
| Beat/miss история | `GET /stock/earnings?symbol=` | `estimate`, `actual`, `surprise`, `surprisePercent` по кварталам. **Внимание: `period` = конец фискального квартала, НЕ дата публикации** |
| Карточка компании | `GET /stock/profile2?symbol=` | name, exchange, currency, ipo, marketCap, shareOutstanding, industry, logo, weburl |
| Консенсус аналитиков | `GET /stock/recommendation?symbol=` | strongBuy/buy/hold/sell/strongSell в динамике по месяцам |
| Новости | `GET /company-news?symbol=&from=&to=` | headline, summary, source, url, datetime |
| Метрики/маржи | `GET /stock/metric?symbol=&metric=all` | grossMargin, operatingMargin, netMargin, ROE/ROA/ROIC, P/E, EV/EBITDA — годовые И квартальные ряды |

**Лимиты:** 60 запросов/мин (free). База: `https://finnhub.io/api/v1`. Auth: `&token=KEY`.

**Не доступно на free (подтверждённые 403):** `stock/price-target`, forward EPS/revenue estimates (дальние), OHLC (`stock/candle`). Прайс-таргет выкинут. OHLC — у Tiingo. Дальний forward — сознательно не берём (§11).

**✅ РИСК СНЯТ — история календаря доступна.** Проверено вживую: `calendar/earnings?from=2024-10-01&to=2024-10-31` на free возвращает данные. «1 month» в маркетинге = окно одного запроса, а не запрет на прошлое. Следствие: даты прошлых публикаций бэкфилим помесячным циклом (3 года истории = 36 запросов ≈ минута при лимите 60/мин), и история реакций доступна сразу, без накопления «вперёд».

### Tiingo — цены (OHLC для реакции на отчёт)

| Что | Эндпоинт | Отдаёт |
|---|---|---|
| Дневной OHLC | `GET /tiingo/daily/{ticker}/prices?startDate=&endDate=` | date, open/high/low/close, volume, **adjOpen/adjHigh/adjLow/adjClose**, adjVolume, divCash, splitFactor |
| Метадата тикера | `GET /tiingo/daily/{ticker}` | name, description, startDate, endDate, exchangeCode |

**Лимиты:** ~50 уникальных символов/час, ~1000 запросов/день (free; Tiingo пишет, что цифры приблизительны). История 30+ лет. База: `https://api.tiingo.com`. Auth: `Authorization: Token KEY` или `?token=KEY`.

**Критично — adjusted vs raw:** `close` и `adjClose` расходятся в прошлом (корректировка на дивиденды/сплиты) и сходятся в настоящем. Для реакции используем **только adjusted-серию**, иначе сплит/дивиденд исказит историю.

**SPY:** индексный ETF тянется тем же эндпоинтом как обычный тикер — нужен для рыночно-нейтральной реакции (§6).

### Покрытие
- Ближайший forward (консенсус на выходящий квартал) → Finnhub `calendar/earnings`, все тикеры.
- Прошлые beat/miss → Finnhub `stock/earnings`.
- Цена, реакция, рыночный фон (SPY) → Tiingo.
- Дыр нет; единственный вопрос — глубина истории календаря (тест выше).

---

## 2. Три «работы» продукта (user stories)

**Job 1 — «Что выходит на этой неделе и чего ждут».**
> Как трейдер, я хочу календарь ближайших отчётов по watchlist (и по рынку) с датой, BMO/AMC и консенсусом EPS/выручки, чтобы спланировать подготовку.

**Job 2 — «Что за компания и как она обычно отчитывается».**
> Как трейдер, открыв карточку тикера, я хочу за 10 секунд понять: чем занимается компания, маржи и их динамику, консенсус аналитиков и историю «бил/не бил» по кварталам.

**Job 3 — «Как цена реагирует на beat/miss» (killer-фича).**
> Как трейдер, я хочу по каждому прошлому отчёту видеть surprise % И реакцию цены (в т.ч. очищенную от движения рынка), чтобы понять: тикер торгуется «по факту» или «по ожиданиям», и стоит ли входить в отчёт.

Job 1 + Job 2 = Phase 1 (MVP). Job 3 = Phase 2 (данные-пайплайн — дифференциатор в резюме).

---

## 3. Экраны

**S1 — Календарь (главная).** Сетка по дням (CSS grid, без react-big-calendar). Переключатель Watchlist/Рынок. Карточка события: тикер, лого, BMO/AMC-бейдж, EPS est, Rev est. Клик → карточка компании.

**S2 — Карточка компании.** Хедер: лого, имя, биржа, отрасль, market cap. Блоки: описание (Tiingo), спарклайны маржи (recharts, данные Finnhub metric), стек-бар консенсуса, **таблица истории отчётов** (квартал, дата публикации, EPS est/actual, surprise %, + Phase 2: реакция % и реакция vs SPY %), лента новостей.

**S3 — Разбор реакции (Phase 2).** Скаттер (recharts): X = surprise %, Y = реакция % (переключатель: сырая / vs SPY). Точка = отчёт. Виден паттерн: «бьёт и растёт» vs sell-the-news.

**S4 — Watchlist (Phase 2).** Добавить/убрать тикеры. Phase 1 — список в конфиге/таблице без логина; users/JWT — Phase 2.

---

## 4. Архитектура (модульный монолит: domain / adapters / pipeline)

**Репозиторий: один, монорепо** — бот (Phase MVP-0), сайт и всё остальное это один и тот же `finnhub`-клиент и один и тот же домен `calendar`/`company`, просто разные потребители (`adapters/telegram` vs `httpapi`+`frontend`). Раздельные репо создали бы только боль синхронизации без выгоды.

Принцип: **простая архитектура с осознанными границами**. Никаких микросервисов, очередей и CQRS — и это защищаемая позиция (см. ADR). Именование пакетов делает паттерн «порты и адаптеры» узнаваемым с первого взгляда на дерево — важно и для поддержки, и для того, кто откроет репозиторий с резюме.

```
backend/
  cmd/
    server/main.go            # ОДИН бинарь: chi http + cron в том же процессе (--no-cron флаг)
  internal/
    domain/                   # ЯДРО: чистые типы + интерфейсы (порты). Ноль внешних зависимостей.
      calendar/               #   Event; интерфейс EventSource
      company/                #   Company, Margins, EarningsRow; интерфейсы ProfileSource, FundamentalsSource
      reaction/                #  расчёт §6 — ЧИСТАЯ ФУНКЦИЯ (bars in → reaction out), 0 зависимостей
    adapters/                 # РЕАЛИЗАЦИИ портов
      finnhub/                #   EventSource, ProfileSource, FundamentalsSource, NewsSource
      tiingo/                 #   PriceSource (OHLC + metadata), включая SPY
      postgres/               #   репозитории: sqlc-генерация из SQL (queries/*.sql -> код)
    pipeline/                 # cron-джобы как first-class citizen: это главная «история» проекта
      backfill.go             #   разовый: календарь в прошлое (помесячно), цены (30 лет одним запросом)
      incremental.go          #   ежедневный: календарь вперёд, цены+SPY, профили/маржи/консенсус/новости
      recompute.go            #   пересчёт earnings_reaction после апдейтов
    httpapi/                  # хендлеры + DTO собственного API, CORS
    httpx/                    # общий http-клиент: rate limiter (x/time/rate), retry+backoff 429/5xx
    config/
  migrations/                 # golang-migrate: *.up.sql / *.down.sql
  sqlc.yaml                   # конфиг sqlc; queries/ рядом с adapters/postgres
docs/
  adr/                        # Architecture Decision Records, по 10-15 строк каждый:
    0001-modular-monolith.md  #   почему не микросервисы
    0002-two-providers.md     #   почему Finnhub+Tiingo, история отказа от FMP (вайтлист)
    0003-adjusted-prices.md   #   почему adjusted-серия обязательна
    0004-sqlc-no-orm.md       #   почему sqlc, а не GORM/руками
    0005-cron-not-queue.md    #   почему cron в процессе, а не Kafka/RabbitMQ
frontend/
  features/                   # по фичам, не по типам файлов
    calendar/                 #   CalendarGrid + хук useCalendar + типы
    company/                  #   CompanyCard + useCompany
    reaction/                 #   ReactionChart + useReaction
  shared/                     #   ui-примитивы, api-клиент (fetch), общие типы
  app/                        # App Router: маршруты тонкие, собирают features
.github/workflows/ci.yml     # golangci-lint + go test + sqlc diff + vitest
Makefile                      # make up = docker-compose + migrate + seed; make test; make lint
```

Правила зависимостей (то, что рассказываешь на собеседовании за 30 секунд):
- `domain` не импортирует НИЧЕГО из adapters/pipeline/httpapi. Интерфейсы объявлены в domain рядом с потребителем — идиоматичный Go.
- `adapters` реализуют интерфейсы domain; знают про JSON провайдеров и SQL, домен — нет.
- `reaction` — чистая функция: идеально тестируется таблично (BMO/AMC/unknown/праздники) без моков.
- Замена провайдера = один пакет в adapters. Доказано фактом: миграция плана FMP → Finnhub+Tiingo не тронула домен.

**Зависимости Go:** chi/v5, pgx/v5, **sqlc** (codegen, принят), robfig/cron/v3, rs/cors, joho/godotenv, golang.org/x/time/rate, golang-migrate (cli), slog (stdlib), golang-jwt/jwt/v5 (Phase 2); тесты: testify (+ testcontainers-go — опция для интеграционных).
**Зависимости Front:** next/react, @tanstack/react-query, zustand, date-fns, recharts, Tailwind; тесты: vitest, @testing-library, msw. HTTP — нативный fetch. Без ORM.

**Сознательно НЕ добавляем** (и это тоже пункт резюме): микросервисы, Kafka/RabbitMQ (cron покрывает всё), CQRS/event sourcing, gRPC внутри себя, Kubernetes. На «почему?» — ответ уровня сеньора в ADR.

**Дешёвые добавки с высоким сигналом:** README с ASCII-диаграммой потока данных (providers → pipeline → postgres → api → next), CI-бейджи, .env.example, seed-скрипт, graceful shutdown, request-id в логах.

---

## 5. Схема БД (PostgreSQL)

Все загрузки — идемпотентные upsert'ы: `INSERT ... ON CONFLICT ... DO UPDATE`. Запросы живут в `queries/*.sql`, Go-код генерит sqlc.

```sql
CREATE TABLE company_profile (
    ticker            TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    exchange          TEXT,
    currency          TEXT,
    industry          TEXT,
    ipo_date          DATE,
    market_cap        NUMERIC,
    share_outstanding NUMERIC,
    logo_url          TEXT,
    web_url           TEXT,
    description       TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Календарь отчётов: ЕДИНСТВЕННЫЙ источник даты публикации и hour.
-- FIX v2: UNIQUE по (ticker, year, quarter) — компании ПЕРЕНОСЯТ даты,
-- report_date обновляемое поле; старый ключ с датой плодил бы дубли.
CREATE TABLE earnings_calendar (
    id               BIGSERIAL PRIMARY KEY,
    ticker           TEXT NOT NULL,
    year             SMALLINT NOT NULL,
    quarter          SMALLINT NOT NULL,
    report_date      DATE NOT NULL,
    hour             TEXT,             -- 'bmo' | 'amc' | ''
    eps_estimate     NUMERIC,
    eps_actual       NUMERIC,
    revenue_estimate NUMERIC,
    revenue_actual   NUMERIC,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticker, year, quarter)
);
CREATE INDEX idx_calendar_date ON earnings_calendar (report_date);

-- История beat/miss. period = конец фискального квартала (НЕ дата публикации).
CREATE TABLE company_earnings (
    id               BIGSERIAL PRIMARY KEY,
    ticker           TEXT NOT NULL,
    period           DATE NOT NULL,
    year             SMALLINT,
    quarter          SMALLINT,
    eps_estimate     NUMERIC,
    eps_actual       NUMERIC,
    surprise         NUMERIC,
    surprise_percent NUMERIC,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticker, period)
);

CREATE TABLE recommendations (
    id          BIGSERIAL PRIMARY KEY,
    ticker      TEXT NOT NULL,
    period      DATE NOT NULL,
    strong_buy  SMALLINT, buy SMALLINT, hold SMALLINT,
    sell        SMALLINT, strong_sell SMALLINT,
    UNIQUE (ticker, period)
);

CREATE TABLE price_daily (
    id           BIGSERIAL PRIMARY KEY,
    ticker       TEXT NOT NULL,        -- включая 'SPY'
    date         DATE NOT NULL,
    open NUMERIC, high NUMERIC, low NUMERIC, close NUMERIC, volume BIGINT,
    adj_open NUMERIC, adj_high NUMERIC, adj_low NUMERIC, adj_close NUMERIC, adj_volume BIGINT,
    div_cash     NUMERIC,
    split_factor NUMERIC,
    UNIQUE (ticker, date)
);
CREATE INDEX idx_price_ticker_date ON price_daily (ticker, date);

-- Реакция. FIX v2: report_date берётся из earnings_calendar
-- (join по ticker+year+quarter), НЕ из company_earnings.period.
CREATE TABLE earnings_reaction (
    id                   BIGSERIAL PRIMARY KEY,
    ticker               TEXT NOT NULL,
    year                 SMALLINT NOT NULL,
    quarter              SMALLINT NOT NULL,
    report_date          DATE NOT NULL,
    hour                 TEXT,
    surprise_percent     NUMERIC,
    price_before         NUMERIC,      -- adjClose
    price_after          NUMERIC,      -- adjClose
    reaction_percent     NUMERIC,      -- сырая реакция
    spy_percent          NUMERIC,      -- движение SPY в том же окне
    reaction_vs_spy      NUMERIC,      -- reaction_percent - spy_percent
    computed_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticker, year, quarter)
);

-- Phase 2
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE watchlist (
    id      BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    ticker  TEXT NOT NULL,
    UNIQUE (user_id, ticker)
);
```

---

## 6. Расчёт реакции на отчёт (сердце Job 3)

**Шаг 0 (FIX v2): определить дату публикации.** `company_earnings.period` — это конец фискального квартала (STZ: period 2026-06-30, отчёт вышел 1 июля — проверено на реальных данных). Реакцию считаем от **report_date из `earnings_calendar`**, joined по `(ticker, year, quarter)`. Нет строки в календаре → реакцию по этому кварталу не считаем (честный NULL, не гадание).

**Шаг 1: окно реакции по `hour`.** «Сессия» — фактические строки `price_daily`, не календарные дни (выходные/праздники):
- **BMO** — отчёт до открытия дня D: `before = adjClose[последняя сессия < D]`, `after = adjClose[D]`.
- **AMC** — отчёт после закрытия дня D: `before = adjClose[D]`, `after = adjClose[первая сессия > D]`.
- **Unknown ('')** — безопасное окно: `before = adjClose[последняя сессия < D]`, `after = adjClose[первая сессия > D]`.

**Шаг 2: метрики.**
```
reaction_percent = (after - before) / before * 100
spy_percent      = то же самое для SPY в том же окне дат
reaction_vs_spy  = reaction_percent - spy_percent
```
`reaction_vs_spy` — главная метрика: очищает реакцию от общерыночного движения (отчёт в день обвала рынка перестаёт врать). Стоит одну колонку и один дополнительный тикер (SPY) в дневном инкременте.

Позже (Phase 2.5, не сейчас): реакция по гэпу открытия (`adjOpen`) отдельно от close-to-close; нормализация на среднюю дневную волатильность тикера («двинулся на 5%, но он и так ходит по 3%»).

---

## 7. Собственный API-контракт (Go → Next.js)

Фронт никогда не ходит в Finnhub/Tiingo напрямую — только в Go (ключи не текут, кэш общий).

```
GET /api/calendar?from=2026-07-01&to=2026-07-31&scope=watchlist|market
  → [{ ticker, name, logoUrl, reportDate, hour, quarter, year,
       epsEstimate, revenueEstimate }]

GET /api/company/{ticker}
  → { profile: {...},
      margins: { gross: [...], operating: [...], net: [...] },   // квартальные ряды
      recommendation: { strongBuy, buy, hold, sell, strongSell },
      earnings: [{ period, quarter, year, epsEstimate, epsActual, surprisePercent,
                   reportDate|null, reactionPercent|null, reactionVsSpy|null }],
      news: [{ headline, source, url, datetime }] }

GET /api/company/{ticker}/reaction        # Phase 2
  → [{ reportDate, quarter, year, surprisePercent,
       reactionPercent, spyPercent, reactionVsSpy, hour }]

# Phase 2
POST /api/auth/register   POST /api/auth/login
GET  /api/watchlist       POST /api/watchlist    DELETE /api/watchlist/{ticker}
```

TS-типы зеркалят контракт; данные — react-query хуки (`useCalendar`, `useCompany`, `useReaction`); UI-состояние — zustand.

---

## 8. Кэш и rate-limit стратегия

Данные тянет cron (в процессе server'а), хендлеры читают только Postgres.

| Задача | Частота | Источник |
|---|---|---|
| Календарь на 30 дней вперёд | 1×/день | Finnhub calendar |
| Бэкфилл календаря в прошлое (подтверждено, §1) | разово, помесячный цикл (3 года = 36 запросов) | Finnhub calendar |
| Профиль + маржи + консенсус | 1×/день, по watchlist | Finnhub |
| Beat/miss история | 1×/день, по watchlist | Finnhub earnings |
| Новости | каждые 6 ч, по watchlist | Finnhub news |
| Цены watchlist + **SPY** (инкремент) | 1×/день после закрытия US | Tiingo |
| Цены (бэкфилл истории) | разово при добавлении тикера | Tiingo (1 запрос = 30 лет) |
| Пересчёт реакций | после апдейта цен/earnings/календаря | внутренний |

Клиентские лимитеры в `httpx`: Finnhub 60/мин, Tiingo ~40 симв/час (запас от заявленных 50), retry с экспоненциальным backoff на 429/5xx. Для watchlist 20–30 тикеров — запас огромный.

**Где упрёшься:** цена по ЛЮБОМУ тикеру мгновенно на клик (вне watchlist) — Tiingo 50/час станет узким местом → очередь фоновой подгрузки или Tiingo Power (~$30/мес). Для личного инструмента — не упрёшься.

---

## 8.5. Phase MVP-0 — Telegram-бот (делаем ПЕРВЫМ, до сайта)

Прежде чем строить Phase 1 (сайт с календарём и карточкой компании), делаем максимально тонкий вертикальный срез: бот, который раз в день утром присылает в Telegram-группу список сегодняшних отчётов по watchlist + краткую инфу о компании + как отчитались в прошлый раз. Это не отдельный проект — это первый работающий кусок той же архитектуры, без БД и без сервера.

**Почему без БД:** для дайджеста по ~20 тикерам данные тянутся live из Finnhub за секунды на каждый прогон — хранить нечего и незачем. Postgres становится обязателен только в Phase 2 (история цен, join'ы, расчёт реакции).

**Почему без сервера:** боту не нужен входящий HTTPS-эндпоинт (он только отправляет `sendMessage`, webhook не нужен — команд для реакции в реальном времени нет). Раз в день запустить бинарник идеально закрывает **GitHub Actions по расписанию (cron)** — бесплатно, без аренды и администрирования.

**Структура (уже в общей структуре репозитория, не отдельная):**
```
cmd/bot/main.go              # один линейный прогон: fetch → format → send → exit
internal/config/             # env: FINNHUB_TOKEN, TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID, WATCHLIST
internal/adapters/finnhub/   # ТЕ ЖЕ 3 метода, что и в общем плане: GetCalendar, GetPastEarnings, GetProfile
internal/adapters/telegram/  # новый адаптер: SendMessage(chatID, text) — один POST-запрос, без SDK
internal/domain/digest/      # чистая функция: события+профили+прошлые earnings → текст сообщения
.github/workflows/
  daily_digest.yml           # cron (UTC) + workflow_dispatch для ручного теста
```

Ключевое: `internal/adapters/finnhub` и доменные типы, написанные для бота, **переиспользуются без изменений** в Phase 1/2 — это не выбрасываемый прототип, а первый слой финальной архитектуры. `internal/domain/digest` позже становится частью `internal/pipeline/daily_digest.go`.

**Логика дайджеста (финальная, без watchlist — фильтруем весь рынок по календарю дня):**

1. **Фильтр «есть ли данные».** Из ответа `calendar/earnings` за сегодняшний день выкидываем тикеры, у которых `epsEstimate` И `revenueEstimate` оба `null` — это в основном closed-end funds и неликвид без покрытия аналитиков (проверено на реальном дне: 15 из 17 тикеров отсеялись именно так, остались только те, что аналитики реально оценивают).

2. **Группировка по времени** (`hour`): `[BMO]` (до открытия) / `[AMC]` (после закрытия) / `[TBD]` (не указано, встречается часто — ~40-50% записей, показываем явно, не молчим).

3. **Тег размера** (по `revenueEstimate`, бесплатный proxy market cap, доп. запросов не требует): `[XL]` от $10 млрд, `[L]` $1-10 млрд, `[M]` $100 млн - $1 млрд, `[S]` меньше $100 млн.

4. **Сигнальные теги, ДО отчёта** (сетап): `[LOSS-EXPECTED]` если `epsEstimate < 0`; усиленный `[TURNAROUND-WATCH]` — тот же случай, с явной пометкой, что если компания отчитается в плюс при ожидаемом убытке — это редкий и обычно сильный разворотный сигнал для микро/малых компаний, потенциальный памп.

5. **Сигнальные теги, ПОСЛЕ отчёта** (когда `epsActual` заполнен, для истории/повторной отправки): `[BEAT]` / `[MISS]` (actual vs estimate); `[TURNAROUND]` — реализовавшийся `TURNAROUND-WATCH` (estimate < 0, actual > 0).

Формат строки дайджеста:
```
[M] [BMO] [LOSS-EXPECTED] [TURNAROUND-WATCH]
SANW — Sanara MedTech
EPS ожидание: -1.63 | Revenue ожидание: $6.6M
```

Без watchlist: календарь дня целиком проходит через фильтр (1), внутри — сортировка по `revenueEstimate` (крупные сверху). В пиковые недели сезона отчётности (сотни тикеров/день) это всё ещё может дать длинный список — если станет нечитаемым, добавить порог на `[S]`/`[M]` (например, схлопывать в компактный список без карточки), но пока не делаем преждевременно — сначала смотрим, насколько живой список от фильтра (1) в реальности короче исходных 300+.

**Шаги:**
1. Ротировать токен бота (был засвечен в чате) в BotFather; секреты (`FINNHUB_TOKEN`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`) — в GitHub Secrets, никогда не в код/чат.
2. Определить watchlist (список тикеров через env).
3. Клиент Finnhub (3 метода) + клиент Telegram (1 метод).
4. `digest`-форматтер: если отчётов на сегодня нет — отдельное короткое сообщение, не молчать.
5. `cmd/bot/main.go`: дата по Варшаве → запрос по watchlist → сборка дайджеста → отправка → выход.
6. `daily_digest.yml`: `cron: "0 6 * * *"` (UTC) + `workflow_dispatch`. **Заметка про DST:** GitHub Actions cron не подстраивается под переход на летнее/зимнее время — 8:00 Варшава это то UTC+1, то UTC+2, значит строку cron нужно вручную поправлять дважды в год (или сразу брать VPS с системным таймером, если критична точность 8:00 круглый год).
7. Локальный тест (`.env`, `go run ./cmd/bot`) → тест ручным запуском workflow в GitHub → бот живёт сам, $0/мес.

---

## 9. Порядок сборки по фазам

**Phase 0 — каркас (0.5–1 день).**
docker-compose (Postgres), golang-migrate + первая миграция, config, chi-сервер с `/health` и graceful shutdown, slog, Next.js скелет, CORS, **CI (GitHub Actions: golangci-lint + go test + vitest)**. `httpx` с limiter'ом, sqlc-конфиг + первый сгенерированный репозиторий. По одному методу в адаптерах finnhub/tiingo + smoke-тест на реальном ключе. Скелет docs/adr с первыми двумя записями.

**Phase 1 — MVP (Job 1 + Job 2).**
1. `finnhub`: calendar, profile, earnings, recommendation, news, metric.
2. `prices`: Tiingo metadata.
3. Миграции всех таблиц кроме reaction/users/watchlist.
4. Cron: календарь + профиль/earnings/консенсус/новости по watchlist (upsert'ы).
5. API: `/api/calendar`, `/api/company/{ticker}`.
6. Front: CalendarGrid (S1), CompanyCard (S2) со спарклайнами (recharts).
7. Тесты: парсеры на фикстурах из реальных дампов STZ/MU, репозитории, хендлеры (httptest), фронт (msw).

**Phase 2 — killer-фича (Job 3).**
1. `prices`: OHLC бэкфилл + дневной инкремент, включая SPY.
2. Бэкфилл календаря в прошлое помесячным циклом (подтверждён, §1) — история реакций доступна сразу.
3. `reaction`: расчёт §6 (join через календарь!), пересчёт-cron.
4. API `/api/company/{ticker}/reaction`, экран S3 (скаттер, переключатель сырая/vs SPY).
5. Логин + watchlist (JWT, экран S4).

**Phase 3 — крипта + алерты.**
Binance/Bybit коннекторы, rule-engine на Go (real-time — твоя сильная зона), .ics-напоминания об отчётах.

---

## 10. Тестирование, CI, деплой

- **Юнит:** парсеры провайдеров на зафиксированных фикстурах (реальные дампы STZ/MU → testdata); логика reaction на синтетике: BMO/AMC/unknown, отчёт в пятницу AMC (реакция в понедельник), праздники, отсутствие строки календаря → NULL.
- **Интеграция:** репозитории на Postgres (go-sqlmock или testcontainers-go), хендлеры через httptest.
- **Фронт:** msw-моки твоего API.
- **CI:** GitHub Actions — golangci-lint, go test, vitest на каждый push. Зелёные чеки в публичном репо — дешёвые очки в портфолио.
- **Деплой (Phase 1+, когда появляется сайт и Postgres):** Railway или Fly.io, **один процесс** (server с cron внутри) + managed Postgres. Секреты `FINNHUB_TOKEN`, `TIINGO_TOKEN` — в env платформы. Multi-stage Dockerfile.

### Домен и хостинг — что и когда покупать

| Этап | Нужен сервер? | Нужен домен? | Решение |
|---|---|---|---|
| MVP-0 (бот) | Нет | Нет | GitHub Actions cron — $0 |
| Phase 1 (сайт, показать что работает) | Да, managed | Не обязательно | Railway/Fly.io, бесплатный поддомен (`*.up.railway.app`) |
| Публичный продукт / резюме с брендом | Да | Да | Свой домен + тот же Railway/Fly, либо VPS |

**Домен:** Namecheap, Porkbun или Cloudflare Registrar — $10–15/год за `.com`. Не AWS Route 53 — дороже и избыточно для личного домена.

**Сервер — сознательно НЕ AWS.** AWS оправдан при масштабе, автоскейлинге и командной инфраструктуре — ничего из этого здесь нет. Для соло-проекта AWS означает сложную консоль, риск неожиданных счетов (забытый инстанс = счёт), и время на IAM/VPC вместо кода продукта. Вместо этого:
- **Railway / Fly.io** — git push → задеплоилось, managed Postgres из коробки, бесплатный tier закрывает личный проект. Рекомендация по умолчанию для Phase 1.
- **Hetzner VPS** (~4–5€/мес) — если хочется полного контроля (systemd-таймер вместо GitHub Actions, свой Postgres, дешевле в долгую) и/или отдельная строчка в резюме «сам настраивал VPS/Nginx/systemd». Не обязательно для этого проекта, но валидный апгрейд.


---

## 11. Осознанно НЕ делаем (и почему)

- **Дальний forward (1–3 года)** — премиум везде (Finnhub платно, FMP вайтлист ~85 тикеров, Tiingo платно). Для торговли на квартальном отчёте не нужен: решающая цифра — консенсус на ближайший отчёт, он в `calendar/earnings` по всем тикерам. FMP как третий сервис выкинут именно поэтому.
- **Price target** — 403 на Finnhub free, не критичен.
- **Company guidance** — бесплатно нигде; при нужде — руками по watchlist.
- **Google Calendar OAuth** — времена отчётов это окна BMO/AMC, не минута; хватит `.ics`.
- **Микросервисы** — изоляция достигнута пакетами, не сетью.
- **Intraday-цены** — дневной реакции достаточно для паттерна; intraday = платные данные и другой класс сложности.