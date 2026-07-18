/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import {
  ChevronLeft,
  ChevronRight,
  Gift,
  QrCode,
  UserPlus,
  type LucideIcon,
} from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getSelf } from '@/lib/api'
import { formatQuota } from '@/lib/format'

import { getInvitedUsers } from '../wallet/api'
import { TransferDialog } from '../wallet/components/dialogs/transfer-dialog'
import { useAffiliate } from '../wallet/hooks'
import type { UserWalletData } from '../wallet/types'

const PAGE_SIZE = 10

type SummaryCard = {
  label: string
  value: string
  icon: LucideIcon
}

function formatCreatedAt(timestamp: number): string {
  if (!timestamp) return '-'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp * 1000))
}

export function Invitation() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)
  const {
    affiliateLink,
    loading: linkLoading,
    transferQuota,
    transferring,
  } = useAffiliate()

  const userQuery = useQuery({
    queryKey: ['invitation', 'self'],
    queryFn: async () => {
      const response = await getSelf()
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load invitation data'))
      }
      return response.data as UserWalletData
    },
  })

  const invitedUsersQuery = useQuery({
    queryKey: ['invitation', 'users', page],
    queryFn: async () => {
      const response = await getInvitedUsers(page, PAGE_SIZE)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load invited users'))
      }
      return response.data
    },
  })

  const user = userQuery.data
  const invitedPage = invitedUsersQuery.data
  const totalPages = Math.max(
    1,
    Math.ceil((invitedPage?.total ?? 0) / PAGE_SIZE)
  )
  const loading = userQuery.isLoading || linkLoading

  const handleTransfer = async (quota: number) => {
    const success = await transferQuota(quota)
    if (success) {
      await userQuery.refetch()
    }
    return success
  }

  const renderInvitedUsers = () => {
    if (invitedUsersQuery.isLoading) {
      return (
        <div className='space-y-3'>
          {['row-1', 'row-2', 'row-3', 'row-4'].map((key) => (
            <Skeleton key={key} className='h-11 w-full' />
          ))}
        </div>
      )
    }

    if (invitedUsersQuery.isError) {
      return (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{t('Failed to load invited users')}</EmptyTitle>
            <EmptyDescription>
              {invitedUsersQuery.error.message}
            </EmptyDescription>
          </EmptyHeader>
          <Button
            variant='outline'
            onClick={() => invitedUsersQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </Empty>
      )
    }

    if ((invitedPage?.items.length ?? 0) === 0) {
      return (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{t('No invited users yet')}</EmptyTitle>
            <EmptyDescription>
              {t('Share your referral link to invite your first user.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )
    }

    return (
      <>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('User ID')}</TableHead>
              <TableHead>{t('Username')}</TableHead>
              <TableHead>{t('Display Name')}</TableHead>
              <TableHead className='text-right'>
                {t('Registration Time')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {invitedPage?.items.map((item) => (
              <TableRow key={item.id}>
                <TableCell className='font-mono'>{item.id}</TableCell>
                <TableCell>{item.username}</TableCell>
                <TableCell>{item.display_name || '-'}</TableCell>
                <TableCell className='text-right'>
                  {formatCreatedAt(item.created_at)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <div className='mt-4 flex items-center justify-between gap-3'>
          <p className='text-muted-foreground text-sm'>
            {t('{{count}} invited users', {
              count: invitedPage?.total ?? 0,
            })}
          </p>
          <div className='flex items-center gap-2'>
            <Button
              variant='outline'
              size='icon-sm'
              disabled={page <= 1}
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              aria-label={t('Previous page')}
            >
              <ChevronLeft aria-hidden='true' />
            </Button>
            <span className='text-sm tabular-nums'>
              {page} / {totalPages}
            </span>
            <Button
              variant='outline'
              size='icon-sm'
              disabled={page >= totalPages}
              onClick={() =>
                setPage((current) => Math.min(totalPages, current + 1))
              }
              aria-label={t('Next page')}
            >
              <ChevronRight aria-hidden='true' />
            </Button>
          </div>
        </div>
      </>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invitation')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
          <div className='grid gap-4 md:grid-cols-3'>
            {(
              [
                {
                  label: t('Invites'),
                  value: String(user?.aff_count ?? 0),
                  icon: UserPlus,
                },
                {
                  label: t('Pending'),
                  value: formatQuota(user?.aff_quota ?? 0),
                  icon: Gift,
                },
                {
                  label: t('Total Earned'),
                  value: formatQuota(user?.aff_history_quota ?? 0),
                  icon: Gift,
                },
              ] satisfies SummaryCard[]
            ).map(({ label, value, icon: Icon }) => (
              <Card key={String(label)}>
                <CardContent className='flex items-center gap-3 p-5'>
                  <div className='bg-primary/10 text-primary flex size-10 items-center justify-center rounded-full'>
                    <Icon className='size-5' aria-hidden='true' />
                  </div>
                  <div>
                    <p className='text-muted-foreground text-sm'>{label}</p>
                    {loading ? (
                      <Skeleton className='mt-1 h-7 w-20' />
                    ) : (
                      <p className='text-2xl font-semibold tabular-nums'>
                        {value}
                      </p>
                    )}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <QrCode className='size-5' aria-hidden='true' />
                {t('Referral Link')}
              </CardTitle>
            </CardHeader>
            <CardContent className='grid gap-6 md:grid-cols-[minmax(0,1fr)_220px] md:items-center'>
              <div className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Share this link. New users who register through it will appear in your invited users list.'
                  )}
                </p>
                <div className='flex flex-wrap gap-2'>
                  <Input
                    value={affiliateLink}
                    readOnly
                    className='min-w-0 flex-1 font-mono text-xs'
                    aria-label={t('Referral Link')}
                  />
                  <CopyButton
                    value={affiliateLink}
                    disabled={!affiliateLink}
                    tooltip={t('Copy referral link')}
                    aria-label={t('Copy referral link')}
                  />
                  {(user?.aff_quota ?? 0) > 0 ? (
                    <Button onClick={() => setTransferDialogOpen(true)}>
                      {t('Transfer to Balance')}
                    </Button>
                  ) : null}
                </div>
              </div>
              <div className='mx-auto flex size-[220px] items-center justify-center rounded-xl border bg-white p-3'>
                {affiliateLink ? (
                  <QRCodeSVG value={affiliateLink} size={190} level='M' />
                ) : (
                  <Skeleton className='size-[190px]' />
                )}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Invited Users')}</CardTitle>
            </CardHeader>
            <CardContent>{renderInvitedUsers()}</CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
      <TransferDialog
        open={transferDialogOpen}
        onOpenChange={setTransferDialogOpen}
        onConfirm={handleTransfer}
        availableQuota={user?.aff_quota ?? 0}
        transferring={transferring}
      />
    </SectionPageLayout>
  )
}
