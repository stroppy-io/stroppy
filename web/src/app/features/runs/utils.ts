import { Status } from '@/proto/panel/types_pb.ts'

export const getStatusLabel = (status: Status, t: (key: string) => string) => {
  switch (status) {
    case Status.COMPLETED:
      return t('status.completed')
    case Status.RUNNING:
      return t('status.running')
    case Status.FAILED:
      return t('status.failed')
    case Status.CANCELLED:
      return t('status.cancelled')
    case Status.IDLE:
      return t('status.pending')
    default:
      return t('status.unknown')
  }
}

export const getStatusBadgeVariant = (status: Status): 'default' | 'secondary' | 'destructive' | 'outline' => {
  switch (status) {
    case Status.COMPLETED:
      return 'default'
    case Status.RUNNING:
      return 'secondary'
    case Status.FAILED:
      return 'destructive'
    default:
      return 'outline'
  }
}
