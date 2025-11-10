interface FullscreenLoaderProps {
  label?: string
}

export const FullscreenLoader = ({ label = 'Загрузка' }: FullscreenLoaderProps) => {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background text-foreground/80">
      <div className="h-12 w-12 animate-spin rounded-full border-4 border-muted border-t-primary" />
      <p className="text-sm font-medium tracking-wide">{label}</p>
    </div>
  )
}
