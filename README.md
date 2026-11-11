# VM-LoadBalancer

Сервис выявления и устранения перекосов потребления ресурсов (CPU и RAM) на гипервизорах KVM/libvirt на основе метрик из VictoriaMetrics.

Проект также включает локальную **лабораторию (стенд)** для наглядного запуска и демонстрации балансировки без реального кластера гипервизоров.

---

## Архитектура

Цикл балансировки работает по схеме `Collect -> Analyze -> Plan -> Execute`:

- **`model/`** — доменные модели (`Hypervisor`, `VirtualMachine`) и методы расчёта утилизации CPU/RAM.
- **`internal/collector/`** — HTTP-клиент к VictoriaMetrics (`/api/v1/query`), собирает метрики хостов и VM и склеивает их в единый снимок кластера.
- **`internal/analyzer/`** — оценивает нагрузку каждого гипервизора (`max(CPU%, RAM%)`), выявляет перекосы по порогам `HighThresholdPercent` и `MinSkewPercent`, формирует отсортированные списки `HotHosts` и `ColdHosts`.
- **`internal/planner/`** — моделирует перенос («что будет с источником и приёмником после миграции») и выбирает оптимальную миграцию, минимизирующую пиковую нагрузку.
- **`internal/executor/`** — интерфейс `Executor` и реализации:
  - `DryRunExecutor` — логирует предложенные миграции без реального выполнения.
  - `LabExecutor` — отправляет команду переноса (`POST /migrate`) в локальный симулятор.
- **`cmd/balancer/`** — основной сервис балансировщика (периодический тикер + graceful shutdown).
- **`cmd/lab-generator/`** — симулятор кластера и экспортер метрик в формате `libvirt_exporter` / `node_exporter` для лаборатории.
- **`deployment/`** — `docker-compose.yml` и `scrape.yml` для локального запуска VictoriaMetrics.

---

## Защита от пинг-понга и фантомных перекосов

1. **Предсказание состояния до миграции (`evaluateMove`)**:
   - Проверяет физическую вместимость приёмника по CPU и RAM.
   - Запрещает перенос, если загрузка приёмника после миграции превысит `TargetMaxPercent` (по умолчанию `70%`).
2. **Гистерезис (`MinGainPercent`)**:
   - Запрещает перенос, если приёмник после миграции не останется минимум на `10%` холоднее исходного хоста — исключает переворот перекоса.
3. **Cooldown на виртуальную машину**:
   - Не позволяет повторно мигрировать одну и ту же VM в течение заданного окна (`45s` в лабе), пока метрики в VictoriaMetrics не стабилизируются.
4. **Агрегация `max by (domain)` для `rate(...)`**:
   - В переходном окне после переезда VM исключает двойной учёт CPU со старого и нового гипервизора.

---

## Ожидаемые метрики в VictoriaMetrics

**Гипервизоры:**
- `node_cpu_cores_total{host="..."}`
- `node_memory_total_bytes{host="..."}`

**Виртуальные машины (`libvirt_exporter`):**
- `libvirt_domain_info_virtual_cpus{domain="...",host="..."}`
- `libvirt_domain_info_memory_usage_bytes{domain="...",host="..."}`
- `libvirt_domain_info_maximum_memory_bytes{domain="...",host="..."}`
- `libvirt_domain_info_cpu_time_seconds_total{domain="...",host="..."}`

---

## Быстрый старт с лабораторией

### 1. Поднять VictoriaMetrics в Docker

```bash
docker compose -f deployment/docker-compose.yml up -d
```

Веб-интерфейс VictoriaMetrics (VMUI) будет доступен по адресу `http://localhost:8428/vmui`.

### 2. Запустить симулятор кластера (`lab-generator`)

В первом терминале:

```bash
go run ./cmd/lab-generator
```

Симулятор поднимет HTTP-сервер на `:9177` с 3 гипервизорами (`hv-01`, `hv-02`, `hv-03`) и 5 виртуальными машинами с исходным перекосом на `hv-01`:
- `GET /metrics` — отдаёт метрики для скрапинга VictoriaMetrics (каждые `5s`).
- `POST /migrate` — принимает `{"vm": "...", "target": "..."}` и перемещает VM между гипервизорами в памяти.

Подождите **20–30 секунд**, чтобы VictoriaMetrics собрала достаточно точек для расчёта `rate(...[30s])`.

### 3. Запустить балансировщик

Во втором терминале:

```bash
# Безопасный режим (только логирование плана, без миграций):
go run ./cmd/balancer --dry-run

# Полный цикл с миграциями в лаборатории:
go run ./cmd/balancer --interval 40s
```

#### Флаги `cmd/balancer`:
- `--vm-url` — адрес VictoriaMetrics (по умолчанию `http://localhost:8428`)
- `--lab-url` — адрес лабораторного API (по умолчанию `http://localhost:9177`)
- `--dry-run` — не выполнять миграции, только выводить план в лог (по умолчанию `false`)
- `--interval` — интервал проверки кластера (по умолчанию `15s`)

---
