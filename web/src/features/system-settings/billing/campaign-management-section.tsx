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
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { getSystemOptions, updateSystemOption } from '../api'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import {
  TopupRulesEditor,
  type CampaignRule,
  type TopupPackageRule,
} from '../integrations/topup-rules-editor'
import { normalizeJsonForComparison } from '../integrations/utils'

type CampaignManagementSectionProps = {
  packagesValue: string
  campaignsValue: string
}

type SaveStatus =
  | 'idle'
  | 'saving'
  | 'reading'
  | 'refreshing'
  | 'synced'
  | 'error'

type CampaignPreview = {
  packageID: string
  packageName: string
  payAmount: number
  permanentAmount: number
  bonusAmount: number
  totalAmount: number
}

function parseArray<T>(value: string): T[] {
  try {
    const parsed = JSON.parse(value || '[]')
    return Array.isArray(parsed) ? (parsed as T[]) : []
  } catch {
    return []
  }
}

function normalizeCampaign(
  campaign: CampaignRule,
  packages: TopupPackageRule[]
): CampaignRule {
  const legacyLaunchCampaign =
    campaign.id === 'launch-first-topup-30' &&
    (campaign.legacy_migration_version ?? 0) < 1 &&
    (campaign.eligibility === 'per_campaign' || !campaign.eligibility) &&
    campaign.max_claims_total > 0
  const eligibility = legacyLaunchCampaign
    ? 'per_package'
    : campaign.eligibility
  const participantLimit = legacyLaunchCampaign
    ? campaign.max_participants_total || campaign.max_claims_total
    : (campaign.max_participants_total ?? 0)
  const enabledPackageIDs = new Set(
    packages.filter((item) => item.enabled).map((item) => item.id)
  )
  const applicablePackageCount =
    campaign.package_ids.length === 0
      ? enabledPackageIDs.size
      : new Set(
          campaign.package_ids.filter((packageID) =>
            enabledPackageIDs.has(packageID)
          )
        ).size
  const claimLimit =
    participantLimit > 0
      ? participantLimit *
        Math.max(campaign.max_claims_per_user ?? 1, 1) *
        (eligibility === 'per_package' ? applicablePackageCount : 1)
      : 0
  return {
    ...campaign,
    eligibility,
    max_claims_per_user:
      eligibility === 'per_package' ? 1 : (campaign.max_claims_per_user ?? 1),
    max_claims_per_email:
      eligibility === 'per_package' ? 1 : (campaign.max_claims_per_email ?? 1),
    max_claims_total: claimLimit,
    max_participants_total: participantLimit,
    reservation_minutes: campaign.reservation_minutes ?? 3,
  }
}

function parseCampaignsForSave(
  campaignsValue: string,
  packages: TopupPackageRule[]
) {
  const parsed = JSON.parse(campaignsValue || '[]') as unknown
  if (!Array.isArray(parsed)) {
    throw new Error('Campaign configuration must be an array')
  }
  const campaigns = (parsed as CampaignRule[]).map((campaign) =>
    normalizeCampaign(campaign, packages)
  )
  return JSON.stringify(campaigns)
}

function getCampaignOptionValue(
  data: Awaited<ReturnType<typeof getSystemOptions>>
) {
  return (
    data.data?.find((option) => option.key === 'payment_setting.campaigns')
      ?.value ?? '[]'
  )
}

function calculateCampaignPreviews(
  campaign: CampaignRule,
  packages: TopupPackageRule[]
): CampaignPreview[] {
  const applicablePackageIDs = new Set(campaign.package_ids)
  return packages
    .filter(
      (packageOption) =>
        packageOption.enabled &&
        (applicablePackageIDs.size === 0 ||
          applicablePackageIDs.has(packageOption.id))
    )
    .map((packageOption) => {
      const totalAmount =
        campaign.reward_mode === 'target_total_percent'
          ? Math.ceil(
              (packageOption.pay_amount * (100 + campaign.reward_percent)) / 100
            )
          : packageOption.credit_amount +
            (campaign.fixed_bonus[packageOption.id] ?? 0)
      const bonusAmount = Math.max(0, totalAmount - packageOption.credit_amount)
      return {
        packageID: packageOption.id,
        packageName: packageOption.name,
        payAmount: packageOption.pay_amount,
        permanentAmount: packageOption.credit_amount,
        bonusAmount,
        totalAmount: packageOption.credit_amount + bonusAmount,
      }
    })
}

function formatCampaigns(value: string) {
  try {
    return JSON.stringify(JSON.parse(value || '[]'), null, 2)
  } catch {
    return value
  }
}

export function CampaignManagementSection({
  packagesValue,
  campaignsValue,
}: CampaignManagementSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const packages = useMemo(
    () => parseArray<TopupPackageRule>(packagesValue),
    [packagesValue]
  )
  const [draftCampaignsValue, setDraftCampaignsValue] = useState(campaignsValue)
  const [effectiveCampaignsValue, setEffectiveCampaignsValue] =
    useState(campaignsValue)
  const previousEffectiveValue = useRef(campaignsValue)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle')
  const [saveMessage, setSaveMessage] = useState('')
  const [savedAt, setSavedAt] = useState<number | null>(null)

  const preparedDraftValue = useMemo(() => {
    try {
      return parseCampaignsForSave(draftCampaignsValue, packages)
    } catch {
      return draftCampaignsValue
    }
  }, [draftCampaignsValue, packages])
  const isDirty =
    normalizeJsonForComparison(preparedDraftValue) !==
    normalizeJsonForComparison(effectiveCampaignsValue)
  const draftCampaigns = useMemo(
    () =>
      parseArray<CampaignRule>(draftCampaignsValue).map((campaign) =>
        normalizeCampaign(campaign, packages)
      ),
    [draftCampaignsValue, packages]
  )

  useEffect(() => {
    setDraftCampaignsValue((currentDraft) =>
      normalizeJsonForComparison(currentDraft) ===
      normalizeJsonForComparison(previousEffectiveValue.current)
        ? campaignsValue
        : currentDraft
    )
    setEffectiveCampaignsValue(campaignsValue)
    previousEffectiveValue.current = campaignsValue
  }, [campaignsValue])

  const handleSave = async () => {
    setFieldErrors({})
    setSaveMessage('')
    setSaveStatus('saving')

    let submittedValue: string
    try {
      submittedValue = parseCampaignsForSave(draftCampaignsValue, packages)
    } catch (error) {
      setSaveStatus('error')
      setSaveMessage(error instanceof Error ? error.message : String(error))
      return
    }

    try {
      const response = await updateSystemOption({
        key: 'payment_setting.campaigns',
        value: submittedValue,
      })
      if (!response.success) {
        setFieldErrors(response.field_errors ?? {})
        setSaveStatus('error')
        setSaveMessage(
          response.message || t('Failed to save campaign settings')
        )
        return
      }
      setSavedAt(response.data?.saved_at ?? Math.floor(Date.now() / 1000))

      setSaveStatus('reading')
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      const systemOptions = await queryClient.fetchQuery({
        queryKey: ['system-options'],
        queryFn: getSystemOptions,
        staleTime: 0,
      })
      const readbackValue = getCampaignOptionValue(systemOptions)
      if (
        normalizeJsonForComparison(readbackValue) !==
        normalizeJsonForComparison(submittedValue)
      ) {
        throw new Error(
          t(
            'Authoritative campaign readback does not match the submitted value'
          )
        )
      }

      setSaveStatus('refreshing')
      await queryClient.invalidateQueries({
        queryKey: ['payment-campaign-stats'],
      })
      await queryClient.refetchQueries(
        { queryKey: ['payment-campaign-stats'], exact: true, type: 'active' },
        { throwOnError: true }
      )

      const formattedReadback = formatCampaigns(readbackValue)
      setEffectiveCampaignsValue(formattedReadback)
      setDraftCampaignsValue(formattedReadback)
      setSaveStatus('synced')
      setSaveMessage(
        t('Saved configuration and real-time statistics are synced')
      )
      toast.success(t('Campaign settings saved and verified'))
    } catch (error) {
      setSaveStatus('error')
      setSaveMessage(error instanceof Error ? error.message : String(error))
      toast.error(t('Campaign settings were saved but synchronization failed'))
    }
  }

  const handleReset = () => {
    setDraftCampaignsValue(formatCampaigns(effectiveCampaignsValue))
    setFieldErrors({})
    setSaveMessage('')
    setSaveStatus('idle')
  }

  return (
    <SettingsSection title={t('Campaign Management')}>
      <SettingsPageFormActions
        onSave={handleSave}
        isSaving={
          saveStatus === 'saving' ||
          saveStatus === 'reading' ||
          saveStatus === 'refreshing'
        }
        saveLabel='Save campaign settings'
      />

      <div className='space-y-6'>
        <Card data-card-hover='false'>
          <CardHeader>
            <div className='flex flex-wrap items-center justify-between gap-3'>
              <div>
                <CardTitle>{t('Draft configuration')}</CardTitle>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t(
                    'Draft values are not active until save, authoritative readback, and statistics refresh all succeed.'
                  )}
                </p>
              </div>
              <div className='flex items-center gap-2'>
                <Badge variant={isDirty ? 'secondary' : 'outline'}>
                  {isDirty
                    ? t('Unsaved draft')
                    : t('Matches saved configuration')}
                </Badge>
                <Button
                  type='button'
                  variant='outline'
                  disabled={!isDirty}
                  onClick={handleReset}
                >
                  {t('Reset draft')}
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <TopupRulesEditor
              campaignsOnly
              packagesValue={packagesValue}
              campaignsValue={draftCampaignsValue}
              supportContactsValue='[]'
              campaignFieldErrors={fieldErrors}
              onPackagesChange={() => undefined}
              onCampaignsChange={setDraftCampaignsValue}
              onSupportContactsChange={() => undefined}
            />
          </CardContent>
        </Card>

        <Card data-card-hover='false'>
          <CardHeader>
            <div className='flex flex-wrap items-center justify-between gap-3'>
              <div>
                <CardTitle>{t('Save and synchronization status')}</CardTitle>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {saveMessage ||
                    t('No save has been verified in this page session.')}
                </p>
              </div>
              <Badge
                variant={saveStatus === 'synced' ? 'default' : 'secondary'}
              >
                {t(saveStatus)}
              </Badge>
            </div>
          </CardHeader>
          <CardContent className='grid gap-3 text-sm sm:grid-cols-2'>
            <div>
              <div className='text-muted-foreground'>{t('Last save time')}</div>
              <div className='mt-1 font-medium'>
                {savedAt ? new Date(savedAt * 1000).toLocaleString() : '—'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('Readback status')}
              </div>
              <div className='mt-1 font-medium'>
                {saveStatus === 'synced'
                  ? t('Verified against system-options')
                  : t('Not verified')}
              </div>
            </div>
          </CardContent>
        </Card>

        <Card data-card-hover='false'>
          <CardHeader>
            <CardTitle>{t('Four-package arrival preview')}</CardTitle>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Preview uses draft rules and the same rounding formula as the server. It is not live statistics.'
              )}
            </p>
          </CardHeader>
          <CardContent className='space-y-5'>
            {draftCampaigns.map((campaign) => (
              <div key={campaign.id} className='space-y-2'>
                <div className='flex flex-wrap items-center gap-2'>
                  <span className='font-medium'>{campaign.name}</span>
                  <Badge variant={campaign.enabled ? 'default' : 'secondary'}>
                    {campaign.enabled ? t('Enabled') : t('Disabled')}
                  </Badge>
                  <span className='text-muted-foreground text-xs'>
                    {t('{{days}} day bonus validity', {
                      days: campaign.valid_days,
                    })}
                  </span>
                </div>
                <div className='overflow-x-auto rounded-md border'>
                  <table className='w-full min-w-[42rem] text-sm'>
                    <thead className='bg-muted/50'>
                      <tr>
                        <th className='px-3 py-2 text-left'>{t('Package')}</th>
                        <th className='px-3 py-2 text-right'>{t('Pay')}</th>
                        <th className='px-3 py-2 text-right'>
                          {t('Permanent balance')}
                        </th>
                        <th className='px-3 py-2 text-right'>
                          {t('Campaign bonus')}
                        </th>
                        <th className='px-3 py-2 text-right'>
                          {t('Total arrival')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {calculateCampaignPreviews(campaign, packages).map(
                        (preview) => (
                          <tr key={preview.packageID} className='border-t'>
                            <td className='px-3 py-2'>{preview.packageName}</td>
                            <td className='px-3 py-2 text-right'>
                              ¥{preview.payAmount.toFixed(2)}
                            </td>
                            <td className='px-3 py-2 text-right'>
                              ¥{preview.permanentAmount.toFixed(2)}
                            </td>
                            <td className='px-3 py-2 text-right'>
                              ¥{preview.bonusAmount.toFixed(2)}
                            </td>
                            <td className='px-3 py-2 text-right font-medium'>
                              ¥{preview.totalAmount.toFixed(2)}
                            </td>
                          </tr>
                        )
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card data-card-hover='false'>
          <CardHeader>
            <CardTitle>{t('Currently effective configuration')}</CardTitle>
            <p className='text-muted-foreground text-sm'>
              {t(
                'This read-only value comes from the latest system-options response and is separate from the draft above.'
              )}
            </p>
          </CardHeader>
          <CardContent>
            <pre className='bg-muted/40 max-h-[32rem] overflow-auto rounded-md border p-4 text-xs'>
              {formatCampaigns(effectiveCampaignsValue)}
            </pre>
          </CardContent>
        </Card>
      </div>
    </SettingsSection>
  )
}
