import { Link } from 'react-router-dom'
import { TopRunsLeaderboard } from '@/app/components/dashboard/top-runs-leaderboard'
import { useTopRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'

type SectionItem = {
  title: string
  description: string
}

export const LandingHomePage = () => {
  const { t } = useTranslation('landing')
  const { data: topRuns = [] } = useTopRunsQuery()
  const capabilities = t('sections.capabilities.items', { returnObjects: true }) as SectionItem[]
  const workflow = t('sections.workflow.steps', { returnObjects: true }) as SectionItem[]
  const integrations = t('sections.integrations.items', { returnObjects: true }) as SectionItem[]
  const audience = t('sections.audience.items', { returnObjects: true }) as string[]
  const reasons = t('sections.reasons.items', { returnObjects: true }) as string[]
  const bundle = t('sections.bundle.items', { returnObjects: true }) as string[]

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-16 px-4 py-16 sm:px-6">
      <section className="rounded-3xl bg-gradient-to-b from-background via-background/90 to-background/70 px-6 py-12 shadow-xl">
        <p className="text-xs uppercase tracking-[0.7em] text-muted-foreground">{t('hero.kicker')}</p>
        <h1 className="mt-4 text-4xl font-bold leading-tight text-foreground sm:text-5xl">{t('hero.title')}</h1>
        <p className="mt-4 max-w-2xl text-lg text-muted-foreground">{t('hero.description')}</p>
        <div className="mt-8 flex flex-wrap gap-3">
          <Link
            to="/register"
            className="rounded-full bg-primary px-6 py-3 text-sm font-semibold uppercase tracking-wide text-primary-foreground transition hover:opacity-90"
          >
            {t('hero.actions.register')}
          </Link>
          <Link
            to="/app/dashboard"
            className="rounded-full border border-border px-6 py-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground transition hover:border-foreground hover:text-foreground"
          >
            {t('hero.actions.openConsole')}
          </Link>
        </div>
      </section>

      <section className="space-y-4">
        <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('sections.overview.title')}</p>
        <div className="rounded-3xl border border-border/50 bg-card/50 px-6 py-6 shadow-inner">
          <p className="text-base text-muted-foreground">{t('sections.overview.description')}</p>
          <p className="mt-4 text-sm font-semibold text-foreground">{t('sections.overview.note')}</p>
        </div>
      </section>

      <section id="features" className="space-y-6">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('sections.capabilities.title')}</p>
          <p className="mt-2 text-base text-muted-foreground">{t('sections.capabilities.description')}</p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          {capabilities.map((item) => (
            <article key={item.title} className="rounded-2xl border border-border/60 bg-background/60 p-5 shadow-sm">
              <h3 className="text-lg font-semibold text-foreground">{item.title}</h3>
              <p className="mt-2 text-sm text-muted-foreground">{item.description}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="space-y-6">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('sections.workflow.title')}</p>
          <p className="mt-2 text-base text-muted-foreground">{t('sections.workflow.description')}</p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          {workflow.map((item) => (
            <article key={item.title} className="rounded-2xl border border-border/60 bg-card/50 p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-primary">{item.title}</h3>
              <p className="mt-2 text-sm text-muted-foreground">{item.description}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="space-y-6">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('sections.integrations.title')}</p>
          <p className="mt-2 text-base text-muted-foreground">{t('sections.integrations.description')}</p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          {integrations.map((item) => (
            <article key={item.title} className="rounded-2xl border border-border/60 bg-background/70 p-5">
              <h3 className="text-base font-semibold text-foreground">{item.title}</h3>
              <p className="mt-2 text-sm text-muted-foreground">{item.description}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="grid gap-6 lg:grid-cols-2">
        <div className="rounded-3xl border border-border/60 bg-card/60 p-6">
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('sections.audience.title')}</p>
          <p className="mt-2 text-base text-muted-foreground">{t('sections.audience.description')}</p>
          <ul className="mt-4 space-y-2 text-sm text-foreground">
            {audience.map((item) => (
              <li key={item} className="flex gap-2">
                <span className="text-primary">•</span>
                <span className="text-muted-foreground">{item}</span>
              </li>
            ))}
          </ul>
        </div>

        <div className="space-y-6">
          <div className="rounded-3xl border border-border/60 bg-background/60 p-6">
            <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('sections.reasons.title')}</p>
            <ul className="mt-4 space-y-2 text-sm text-muted-foreground">
              {reasons.map((item) => (
                <li key={item} className="flex gap-2">
                  <span className="text-primary">•</span>
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="rounded-3xl border border-border/60 bg-background/80 p-6">
            <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('sections.bundle.title')}</p>
            <ul className="mt-4 space-y-2 text-sm text-muted-foreground">
              {bundle.map((item) => (
                <li key={item} className="flex gap-2">
                  <span className="text-primary">•</span>
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </section>

      <section id="tops" className="space-y-6">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('topRuns.title')}</p>
          <p className="mt-2 text-base text-muted-foreground">{t('charts.tps.subtitle')}</p>
        </div>
        <TopRunsLeaderboard runs={topRuns} />
      </section>

      <section className="rounded-3xl border border-border/60 bg-gradient-to-br from-primary/10 via-background to-background px-6 py-10 text-center shadow-lg">
        <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('cta.title')}</p>
        <p className="mt-3 text-lg text-muted-foreground">{t('cta.description')}</p>
        <div className="mt-6 flex flex-wrap justify-center gap-3">
          <Link
            to="/register"
            className="rounded-full bg-primary px-6 py-3 text-sm font-semibold uppercase tracking-wide text-primary-foreground transition hover:opacity-90"
          >
            {t('cta.getStarted')}
          </Link>
          <Link
            to="/login"
            className="rounded-full border border-border px-6 py-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground transition hover:border-foreground hover:text-foreground"
          >
            {t('cta.existingUser')}
          </Link>
        </div>
      </section>
    </div>
  )
}
