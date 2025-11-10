import { Link } from 'react-router-dom'
import { TopRunsLeaderboard } from '@/app/components/dashboard/top-runs-leaderboard'
import { useTopRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'

export const LandingHomePage = () => {
  const { t } = useTranslation('landing')
  const { data: topRuns = [] } = useTopRunsQuery()

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-12 px-4 py-16 sm:px-6">
      <div className="rounded-3xl bg-gradient-to-b from-background via-background/90 to-background/70 px-6 py-12 shadow-xl">
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
      </div>

      <TopRunsLeaderboard runs={topRuns} />
    </section>
  )
}
