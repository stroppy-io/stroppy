const presets = [
  { name: 'OLTP intensive', description: 'Высокий TPS, короткие транзакции, агрессивные ретраи' },
  { name: 'Streaming ingest', description: 'Потоковая запись + аналитические запросы на чтение' },
  { name: 'Disaster rehearsal', description: 'chaos инженерия + переключение на реплики' },
]

export const ConfiguratorPage = () => {
  return (
    <div className="space-y-6">
      <header>
        <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">Конфигуратор</p>
        <h1 className="text-3xl font-semibold text-foreground">Шаблоны нагрузок</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Здесь появится редактор YAML/JSON поверх shadcn/ui и Monaco. Пока зафиксируем список пресетов.
        </p>
      </header>

      <div className="grid gap-4 md:grid-cols-3">
        {presets.map((preset) => (
          <article key={preset.name} className="rounded-2xl border border-dashed border-border/80 p-4">
            <p className="text-sm font-semibold text-foreground">{preset.name}</p>
            <p className="mt-2 text-xs text-muted-foreground">{preset.description}</p>
            <button className="mt-4 text-xs font-semibold uppercase tracking-wide text-foreground underline underline-offset-4">
              Настроить →
            </button>
          </article>
        ))}
      </div>
    </div>
  )
}
