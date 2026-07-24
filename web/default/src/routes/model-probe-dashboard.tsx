/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { ModelProbeDashboard } from '@/features/model-probe'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/model-probe-dashboard')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user) throw redirect({ to: '/sign-in', search: { redirect: '/model-probe-dashboard' } })
    if (auth.user.role < ROLE.ADMIN) throw redirect({ to: '/403' })
  },
  component: ModelProbeDashboard,
})
