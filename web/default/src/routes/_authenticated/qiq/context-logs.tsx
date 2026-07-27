import { createFileRoute, redirect } from '@tanstack/react-router'

import { ContextLogs } from '@/features/qiq/context-logs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/qiq/context-logs')({
  beforeLoad: () => {
    if (useAuthStore.getState().auth.user?.role !== ROLE.SUPER_ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: ContextLogs,
})
