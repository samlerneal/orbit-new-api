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
import { Plus, Trash2 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'

type TopupPackageRule = {
  id: string
  name: string
  description: string
  tag: string
  pay_amount: number
  credit_amount: number
  enabled: boolean
  sort_order: number
  selling_points: string[]
  footer_note: string
  visual_style: string
}

type SupportContactRule = {
  id: string
  type: 'qq' | 'wechat' | 'phone' | 'qrcode'
  value: string
}

type CampaignRule = {
  id: string
  name: string
  banner_title: string
  banner_text: string
  badge_text: string
  enabled: boolean
  starts_at: number
  ends_at: number
  package_ids: string[]
  eligibility: 'per_campaign' | 'per_package' | 'unlimited'
  max_claims_per_user: number
  max_claims_per_email: number
  max_claims_total: number
  max_participants_total: number
  reservation_minutes: number
  reward_mode: 'target_total_percent' | 'fixed_bonus'
  reward_percent: number
  fixed_bonus: Record<string, number>
  rounding_mode: 'ceil_yuan'
  valid_days: number
  stackable: boolean
  priority: number
}

type CampaignStats = {
  campaign_id: string
  limited: boolean
  total: number
  awarded: number
  reserved: number
  remaining: number
  participant_limited: boolean
  participants_total: number
  participants_admitted: number
  participants_reserved: number
  participants_remaining: number
  claims_awarded: number
  claims_reserved: number
}

type CampaignStatsResponse = {
  success: boolean
  message: string
  data: CampaignStats[]
}

const DEFAULT_MAX_CLAIMS_PER_EMAIL = 1
const DEFAULT_MAX_CLAIMS_TOTAL = 100
const DEFAULT_RESERVATION_MINUTES = 3
const SELLING_POINT_SLOT_KEYS = ['first', 'second', 'third'] as const

function parseArray<T>(value: string): T[] {
  try {
    const parsed = JSON.parse(value || '[]')
    return Array.isArray(parsed) ? (parsed as T[]) : []
  } catch {
    return []
  }
}

function normalizeCampaign(campaign: CampaignRule): CampaignRule {
  return {
    ...campaign,
    max_claims_per_email:
      campaign.max_claims_per_email ?? DEFAULT_MAX_CLAIMS_PER_EMAIL,
    max_claims_total: campaign.max_claims_total ?? DEFAULT_MAX_CLAIMS_TOTAL,
    max_participants_total: campaign.max_participants_total ?? 0,
    reservation_minutes:
      campaign.reservation_minutes ?? DEFAULT_RESERVATION_MINUTES,
  }
}

function normalizeLegacyCampaignText(campaign: CampaignRule): CampaignRule {
  if (
    campaign.id === 'launch-first-topup-30' &&
    campaign.banner_title === 'First top-up bonus: 30%' &&
    campaign.banner_text ===
      'Complete your first top-up in this campaign to receive time-limited bonus balance.' &&
    campaign.badge_text === 'First top-up +30%'
  ) {
    return {
      ...campaign,
      banner_title: 'Limited four-package bonus: 30%',
      banner_text:
        'Each account can receive the campaign bonus once per eligible package.',
      badge_text: 'Limited bonus +30%',
    }
  }
  return campaign
}

function toDateTimeLocal(timestamp: number) {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function fromDateTimeLocal(value: string) {
  if (!value) return 0
  const timestamp = new Date(value).getTime()
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1000) : 0
}

type TopupRulesEditorProps = {
  packagesValue: string
  campaignsValue: string
  supportContactsValue: string
  onPackagesChange: (value: string) => void
  onCampaignsChange: (value: string) => void
  onSupportContactsChange: (value: string) => void
}

export function TopupRulesEditor({
  packagesValue,
  campaignsValue,
  supportContactsValue,
  onPackagesChange,
  onCampaignsChange,
  onSupportContactsChange,
}: TopupRulesEditorProps) {
  const { t } = useTranslation()
  const packages = useMemo(
    () =>
      parseArray<TopupPackageRule>(packagesValue).map((p) => ({
        ...p,
        selling_points: Array.isArray(p.selling_points)
          ? p.selling_points
              .filter((point): point is string => typeof point === 'string')
              .slice(0, 3)
          : [],
        footer_note: typeof p.footer_note === 'string' ? p.footer_note : '',
        visual_style:
          typeof p.visual_style === 'string' &&
          ['default', 'recommended', 'popular', 'value'].includes(
            p.visual_style
          )
            ? p.visual_style
            : 'default',
      })),
    [packagesValue]
  )
  const campaigns = useMemo(
    () =>
      parseArray<CampaignRule>(campaignsValue)
        .map(normalizeCampaign)
        .map(normalizeLegacyCampaignText),
    [campaignsValue]
  )
  const supportContacts = useMemo(
    () => parseArray<SupportContactRule>(supportContactsValue),
    [supportContactsValue]
  )
  const { data: campaignStats = [] } = useQuery({
    queryKey: ['payment-campaign-stats'],
    queryFn: async () => {
      const response = await api.get<CampaignStatsResponse>(
        '/api/option/payment-campaigns/stats'
      )
      if (!response.data.success) {
        throw new Error(
          response.data.message || 'Failed to load campaign stats'
        )
      }
      return response.data.data ?? []
    },
    retry: false,
  })
  const campaignStatsById = useMemo(
    () => new Map(campaignStats.map((stats) => [stats.campaign_id, stats])),
    [campaignStats]
  )

  const updatePackage = (index: number, patch: Partial<TopupPackageRule>) => {
    const next = packages.map((item, itemIndex) =>
      itemIndex === index ? { ...item, ...patch } : item
    )
    onPackagesChange(JSON.stringify(next, null, 2))
  }

  const updateCampaign = (index: number, patch: Partial<CampaignRule>) => {
    const next = campaigns.map((item, itemIndex) =>
      itemIndex === index ? { ...item, ...patch } : item
    )
    onCampaignsChange(JSON.stringify(next, null, 2))
  }

  const updateSupportContact = (
    index: number,
    patch: Partial<SupportContactRule>
  ) => {
    const next = supportContacts.map((item, itemIndex) =>
      itemIndex === index ? { ...item, ...patch } : item
    )
    onSupportContactsChange(JSON.stringify(next, null, 2))
  }

  const addSupportContact = () => {
    if (supportContacts.length >= 8) return
    onSupportContactsChange(
      JSON.stringify(
        [
          ...supportContacts,
          { id: `support-${Date.now()}`, type: 'qq', value: '' },
        ],
        null,
        2
      )
    )
  }

  const addCampaign = () => {
    const id = `campaign-${Date.now()}`
    const next: CampaignRule[] = [
      ...campaigns,
      {
        id,
        name: t('New top-up campaign'),
        banner_title: t('Top-up bonus'),
        banner_text: '',
        badge_text: t('Campaign bonus'),
        enabled: false,
        starts_at: 0,
        ends_at: 0,
        package_ids: packages.map((item) => item.id),
        eligibility: 'per_campaign',
        max_claims_per_user: 1,
        max_claims_per_email: DEFAULT_MAX_CLAIMS_PER_EMAIL,
        max_claims_total: DEFAULT_MAX_CLAIMS_TOTAL,
        max_participants_total: 0,
        reservation_minutes: DEFAULT_RESERVATION_MINUTES,
        reward_mode: 'target_total_percent',
        reward_percent: 10,
        fixed_bonus: {},
        rounding_mode: 'ceil_yuan',
        valid_days: 30,
        stackable: false,
        priority: 0,
      },
    ]
    onCampaignsChange(JSON.stringify(next, null, 2))
  }

  const addPackage = () => {
    const id = `package-${Date.now()}`
    const next: TopupPackageRule[] = [
      ...packages,
      {
        id,
        name: t('New top-up package'),
        description: '',
        tag: '',
        pay_amount: 1,
        credit_amount: 1,
        enabled: false,
        sort_order:
          Math.max(0, ...packages.map((item) => item.sort_order || 0)) + 10,
        selling_points: [],
        footer_note: '',
        visual_style: 'default',
      },
    ]
    onPackagesChange(JSON.stringify(next, null, 2))
  }

  return (
    <div className='space-y-6'>
      <div className='space-y-3'>
        <div className='flex items-start justify-between gap-3'>
          <div>
            <h4 className='font-medium'>{t('Customer service contacts')}</h4>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Shown in the refund notice. Leaving the list empty does not disable payment.'
              )}
            </p>
          </div>
          <Button
            type='button'
            variant='outline'
            disabled={supportContacts.length >= 8}
            onClick={addSupportContact}
          >
            <Plus className='size-4' />
            {t('Add contact')}
          </Button>
        </div>
        {supportContacts.length === 0 && (
          <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
            {t('No customer service contact configured.')}
          </div>
        )}
        <div className='space-y-2'>
          {supportContacts.map((contact, index) => (
            <div
              key={contact.id}
              className='grid gap-2 rounded-lg border p-3 sm:grid-cols-[160px_1fr_auto]'
            >
              <select
                aria-label={t('Contact type')}
                className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
                value={contact.type}
                onChange={(event) =>
                  updateSupportContact(index, {
                    type: event.target.value as SupportContactRule['type'],
                  })
                }
              >
                <option value='qq'>{t('QQ')}</option>
                <option value='wechat'>{t('WeChat')}</option>
                <option value='phone'>{t('Phone number')}</option>
                <option value='qrcode'>{t('QR code image')}</option>
              </select>
              <Input
                value={contact.value}
                placeholder={
                  contact.type === 'qrcode'
                    ? t('HTTPS image URL')
                    : t('Contact account or number')
                }
                onChange={(event) =>
                  updateSupportContact(index, { value: event.target.value })
                }
              />
              <Button
                type='button'
                size='icon'
                variant='ghost'
                aria-label={t('Delete contact')}
                onClick={() =>
                  onSupportContactsChange(
                    JSON.stringify(
                      supportContacts.filter(
                        (_item, itemIndex) => itemIndex !== index
                      ),
                      null,
                      2
                    )
                  )
                }
              >
                <Trash2 className='size-4' />
              </Button>
            </div>
          ))}
        </div>
      </div>

      <div className='space-y-3'>
        <div className='flex items-start justify-between gap-3'>
          <div>
            <h4 className='font-medium'>{t('Top-up packages')}</h4>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Edit the text, price and permanent balance for each package.'
              )}
            </p>
          </div>
          <Button type='button' variant='outline' onClick={addPackage}>
            <Plus className='size-4' />
            {t('Add package')}
          </Button>
        </div>
        <div className='grid gap-3 lg:grid-cols-2'>
          {packages.map((item, index) => (
            <Card key={item.id} data-card-hover='false'>
              <CardHeader className='pb-3'>
                <div className='flex items-center justify-between gap-3'>
                  <CardTitle className='text-base'>{item.name}</CardTitle>
                  <div className='flex items-center gap-2'>
                    <Switch
                      checked={item.enabled}
                      onCheckedChange={(enabled) =>
                        updatePackage(index, { enabled })
                      }
                    />
                    <Button
                      type='button'
                      size='icon'
                      variant='ghost'
                      disabled={packages.length <= 1}
                      aria-label={t('Delete package')}
                      onClick={() =>
                        onPackagesChange(
                          JSON.stringify(
                            packages.filter(
                              (_item, itemIndex) => itemIndex !== index
                            ),
                            null,
                            2
                          )
                        )
                      }
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent className='grid gap-3 sm:grid-cols-2'>
                <Field label={t('Name')}>
                  <Input
                    value={item.name}
                    onChange={(event) =>
                      updatePackage(index, { name: event.target.value })
                    }
                  />
                </Field>
                <Field label={t('Tag')}>
                  <Input
                    value={item.tag ?? ''}
                    placeholder={t('Optional')}
                    onChange={(event) =>
                      updatePackage(index, { tag: event.target.value })
                    }
                  />
                </Field>
                <Field label={t('Description')} className='sm:col-span-2'>
                  <Input
                    value={item.description}
                    onChange={(event) =>
                      updatePackage(index, {
                        description: event.target.value,
                      })
                    }
                  />
                </Field>
                <Field label={t('Payment amount (RMB)')}>
                  <Input
                    type='number'
                    min={0.01}
                    step={0.01}
                    value={item.pay_amount}
                    onChange={(event) =>
                      updatePackage(index, {
                        pay_amount: Number(event.target.value),
                      })
                    }
                  />
                </Field>
                <Field label={t('Permanent balance received (RMB)')}>
                  <Input
                    type='number'
                    min={0.01}
                    step={0.01}
                    value={item.credit_amount}
                    onChange={(event) =>
                      updatePackage(index, {
                        credit_amount: Number(event.target.value),
                      })
                    }
                  />
                </Field>
                <Field label={t('Sort order')}>
                  <Input
                    type='number'
                    min={0}
                    step={10}
                    value={item.sort_order}
                    onChange={(event) =>
                      updatePackage(index, {
                        sort_order: Number(event.target.value),
                      })
                    }
                  />
                </Field>
                <Field label={t('Visual style')} className='sm:col-span-2'>
                  <select
                    className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
                    value={item.visual_style || 'default'}
                    onChange={(event) =>
                      updatePackage(index, {
                        visual_style: event.target.value,
                      })
                    }
                  >
                    <option value='default'>{t('Default')}</option>
                    <option value='recommended'>{t('Recommended')}</option>
                    <option value='popular'>{t('Popular')}</option>
                    <option value='value'>{t('Value')}</option>
                  </select>
                </Field>
                {item.selling_points.map((point, pointIndex) => (
                  <Field
                    key={`${item.id}-selling-point-${SELLING_POINT_SLOT_KEYS[pointIndex]}`}
                    label={`${t('Selling point')} ${pointIndex + 1}`}
                    className='sm:col-span-2'
                  >
                    <div className='flex gap-2'>
                      <Input
                        value={point}
                        maxLength={80}
                        onChange={(event) =>
                          updatePackage(index, {
                            selling_points: item.selling_points.map((p, i) =>
                              i === pointIndex ? event.target.value : p
                            ),
                          })
                        }
                      />
                      <Button
                        type='button'
                        size='icon'
                        variant='ghost'
                        aria-label={t('Delete selling point')}
                        onClick={() =>
                          updatePackage(index, {
                            selling_points: item.selling_points.filter(
                              (_p, i) => i !== pointIndex
                            ),
                          })
                        }
                      >
                        <Trash2 className='size-4' />
                      </Button>
                    </div>
                  </Field>
                ))}
                {item.selling_points.length < 3 && (
                  <div className='sm:col-span-2'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        updatePackage(index, {
                          selling_points: [...item.selling_points, ''],
                        })
                      }
                    >
                      <Plus className='size-4' />
                      {t('Add selling point')}
                    </Button>
                  </div>
                )}
                <Field label={t('Footer note')} className='sm:col-span-2'>
                  <Input
                    value={item.footer_note ?? ''}
                    maxLength={120}
                    placeholder={t('Optional footer text')}
                    onChange={(event) =>
                      updatePackage(index, {
                        footer_note: event.target.value,
                      })
                    }
                  />
                </Field>
                <div className='text-muted-foreground text-xs sm:col-span-2'>
                  {t(
                    'Campaign bonus and expiry is managed in campaign settings'
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>

      <div className='space-y-3'>
        <div className='flex items-start justify-between gap-3'>
          <div>
            <h4 className='font-medium'>{t('Top-up campaigns')}</h4>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Campaigns are data-only rules. Turn them on only after checking the preview.'
              )}
            </p>
          </div>
          <Button type='button' variant='outline' onClick={addCampaign}>
            <Plus className='size-4' />
            {t('Add campaign')}
          </Button>
        </div>

        {campaigns.length === 0 && (
          <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
            {t('No top-up campaign configured.')}
          </div>
        )}

        {campaigns.map((campaign, index) => (
          <Card key={campaign.id} data-card-hover='false'>
            <CardHeader className='pb-3'>
              <div className='flex flex-wrap items-center justify-between gap-3'>
                <div className='flex items-center gap-2'>
                  <CardTitle className='text-base'>{campaign.name}</CardTitle>
                  <Badge variant={campaign.enabled ? 'default' : 'secondary'}>
                    {campaign.enabled ? t('Enabled') : t('Disabled')}
                  </Badge>
                </div>
                <div className='flex items-center gap-2'>
                  <Switch
                    checked={campaign.enabled}
                    onCheckedChange={(enabled) =>
                      updateCampaign(index, { enabled })
                    }
                  />
                  <Button
                    type='button'
                    size='icon'
                    variant='ghost'
                    aria-label={t('Delete campaign')}
                    onClick={() =>
                      onCampaignsChange(
                        JSON.stringify(
                          campaigns.filter(
                            (_item, itemIndex) => itemIndex !== index
                          ),
                          null,
                          2
                        )
                      )
                    }
                  >
                    <Trash2 className='size-4' />
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className='grid gap-4 md:grid-cols-2'>
              <Field label={t('Internal campaign name')}>
                <Input
                  value={campaign.name}
                  onChange={(event) =>
                    updateCampaign(index, { name: event.target.value })
                  }
                />
              </Field>
              <Field label={t('Package badge')}>
                <Input
                  value={campaign.badge_text}
                  onChange={(event) =>
                    updateCampaign(index, { badge_text: event.target.value })
                  }
                />
              </Field>
              <Field label={t('Banner title')}>
                <Input
                  value={campaign.banner_title}
                  onChange={(event) =>
                    updateCampaign(index, { banner_title: event.target.value })
                  }
                />
              </Field>
              <Field label={t('Banner description')}>
                <Input
                  value={campaign.banner_text}
                  onChange={(event) =>
                    updateCampaign(index, { banner_text: event.target.value })
                  }
                />
              </Field>
              <Field label={t('Start time')}>
                <Input
                  type='datetime-local'
                  value={toDateTimeLocal(campaign.starts_at)}
                  onChange={(event) =>
                    updateCampaign(index, {
                      starts_at: fromDateTimeLocal(event.target.value),
                    })
                  }
                />
              </Field>
              <Field label={t('End time')}>
                <Input
                  type='datetime-local'
                  value={toDateTimeLocal(campaign.ends_at)}
                  onChange={(event) =>
                    updateCampaign(index, {
                      ends_at: fromDateTimeLocal(event.target.value),
                    })
                  }
                />
              </Field>
              <Field label={t('Eligibility')}>
                <select
                  className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
                  value={campaign.eligibility}
                  onChange={(event) =>
                    updateCampaign(index, {
                      eligibility: event.target
                        .value as CampaignRule['eligibility'],
                      max_claims_per_user:
                        event.target.value === 'unlimited'
                          ? 0
                          : Math.max(campaign.max_claims_per_user, 1),
                    })
                  }
                >
                  <option value='per_campaign'>{t('Once per campaign')}</option>
                  <option value='per_package'>
                    {t('Once per package per account')}
                  </option>
                  <option value='unlimited'>{t('Unlimited')}</option>
                </select>
              </Field>
              <Field label={t('Maximum claims per account (0 = unlimited)')}>
                <Input
                  type='number'
                  min={0}
                  disabled={campaign.eligibility === 'unlimited'}
                  value={campaign.max_claims_per_user}
                  onChange={(event) =>
                    updateCampaign(index, {
                      max_claims_per_user: Number(event.target.value),
                    })
                  }
                />
              </Field>
              <Field
                label={t('Maximum claims per verified email (0 = unlimited)')}
              >
                <Input
                  type='number'
                  min={0}
                  step={1}
                  value={campaign.max_claims_per_email}
                  onChange={(event) =>
                    updateCampaign(index, {
                      max_claims_per_email: Math.max(
                        0,
                        Number(event.target.value)
                      ),
                    })
                  }
                />
              </Field>
              <Field label={t('Total campaign slots (0 = unlimited)')}>
                <Input
                  type='number'
                  min={0}
                  step={1}
                  value={campaign.max_claims_total}
                  onChange={(event) =>
                    updateCampaign(index, {
                      max_claims_total: Math.max(0, Number(event.target.value)),
                    })
                  }
                />
              </Field>
              <Field label={t('Maximum participants (0 = unlimited)')}>
                <Input
                  type='number'
                  min={0}
                  step={1}
                  value={campaign.max_participants_total}
                  onChange={(event) =>
                    updateCampaign(index, {
                      max_participants_total: Math.max(
                        0,
                        Number(event.target.value)
                      ),
                    })
                  }
                />
              </Field>
              <Field label={t('Slot reservation duration (minutes)')}>
                <Input
                  type='number'
                  min={1}
                  step={1}
                  value={campaign.reservation_minutes}
                  onChange={(event) =>
                    updateCampaign(index, {
                      reservation_minutes: Math.max(
                        1,
                        Number(event.target.value)
                      ),
                    })
                  }
                />
              </Field>
              <CampaignStatsPanel stats={campaignStatsById.get(campaign.id)} />
              <Field label={t('Reward calculation')}>
                <select
                  className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
                  value={campaign.reward_mode}
                  onChange={(event) =>
                    updateCampaign(index, {
                      reward_mode: event.target
                        .value as CampaignRule['reward_mode'],
                    })
                  }
                >
                  <option value='target_total_percent'>
                    {t('Payment amount percentage, rounded up')}
                  </option>
                  <option value='fixed_bonus'>
                    {t('Fixed bonus by package')}
                  </option>
                </select>
              </Field>
              <Field label={t('Bonus percentage')}>
                <Input
                  type='number'
                  min={0.01}
                  step={0.01}
                  disabled={campaign.reward_mode !== 'target_total_percent'}
                  value={campaign.reward_percent}
                  onChange={(event) =>
                    updateCampaign(index, {
                      reward_percent: Number(event.target.value),
                    })
                  }
                />
              </Field>
              <Field label={t('Bonus validity (days)')}>
                <Input
                  type='number'
                  min={1}
                  value={campaign.valid_days}
                  onChange={(event) =>
                    updateCampaign(index, {
                      valid_days: Number(event.target.value),
                    })
                  }
                />
              </Field>
              <Field label={t('Priority')}>
                <Input
                  type='number'
                  value={campaign.priority}
                  onChange={(event) =>
                    updateCampaign(index, {
                      priority: Number(event.target.value),
                    })
                  }
                />
              </Field>

              <div className='space-y-2 md:col-span-2'>
                <Label>{t('Eligible packages')}</Label>
                <div className='flex flex-wrap gap-4 rounded-md border p-3'>
                  {packages.map((item) => {
                    const checked = campaign.package_ids.includes(item.id)
                    return (
                      <label
                        key={item.id}
                        className='flex items-center gap-2 text-sm'
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(nextChecked) =>
                            updateCampaign(index, {
                              package_ids: nextChecked
                                ? [...campaign.package_ids, item.id]
                                : campaign.package_ids.filter(
                                    (packageId) => packageId !== item.id
                                  ),
                            })
                          }
                        />
                        {item.name}
                      </label>
                    )
                  })}
                </div>
              </div>

              {campaign.reward_mode === 'fixed_bonus' && (
                <div className='grid gap-3 sm:grid-cols-2 md:col-span-2 lg:grid-cols-4'>
                  {packages.map((item) => (
                    <Field key={item.id} label={`${item.name} ${t('bonus')}`}>
                      <Input
                        type='number'
                        min={0}
                        step={0.01}
                        value={campaign.fixed_bonus[item.id] ?? 0}
                        onChange={(event) =>
                          updateCampaign(index, {
                            fixed_bonus: {
                              ...campaign.fixed_bonus,
                              [item.id]: Number(event.target.value),
                            },
                          })
                        }
                      />
                    </Field>
                  ))}
                </div>
              )}

              <label className='flex items-center gap-3 text-sm md:col-span-2'>
                <Checkbox
                  checked={campaign.stackable}
                  onCheckedChange={(stackable) =>
                    updateCampaign(index, { stackable: stackable === true })
                  }
                />
                {t(
                  'Allow this campaign to stack with lower-priority campaigns'
                )}
              </label>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}

function CampaignStatsPanel({ stats }: { stats?: CampaignStats }) {
  const { t } = useTranslation()
  let totalSlots: number | string = '—'
  let remainingSlots: number | string = '—'
  let participantTotal: number | string = '—'
  let participantRemaining: number | string = '—'
  if (stats) {
    totalSlots = stats.limited ? stats.total : t('Unlimited')
    remainingSlots = stats.limited ? stats.remaining : t('Unlimited')
    participantTotal = stats.participant_limited
      ? stats.participants_total
      : t('Unlimited')
    participantRemaining = stats.participant_limited
      ? stats.participants_remaining
      : t('Unlimited')
  }

  return (
    <div className='space-y-3 rounded-md border p-3 md:col-span-2'>
      <div className='text-muted-foreground text-xs font-medium'>
        {t('Campaign participant stats')}
      </div>
      <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
        <CampaignStat
          label={t('Maximum participants (0 = unlimited)')}
          value={participantTotal}
        />
        <CampaignStat
          label={t('Admitted accounts')}
          value={stats?.participants_admitted ?? '—'}
        />
        <CampaignStat
          label={t('Reserved accounts')}
          value={stats?.participants_reserved ?? '—'}
        />
        <CampaignStat
          label={t('Remaining participant slots')}
          value={participantRemaining}
        />
      </div>
      <div className='text-muted-foreground text-xs font-medium'>
        {t('Claim stats')}
      </div>
      <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
        <CampaignStat label={t('Total slots')} value={totalSlots} />
        <CampaignStat
          label={t('Awarded claims')}
          value={stats?.claims_awarded ?? '—'}
        />
        <CampaignStat
          label={t('Reserved claims')}
          value={stats?.claims_reserved ?? '—'}
        />
        <CampaignStat label={t('Remaining slots')} value={remainingSlots} />
      </div>
    </div>
  )
}

function CampaignStat({
  label,
  value,
}: {
  label: string
  value: number | string
}) {
  return (
    <div>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='mt-1 text-lg font-semibold'>{value}</div>
    </div>
  )
}

function Field({
  label,
  className,
  children,
}: {
  label: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <div className={className}>
      <Label className='mb-1.5 block'>{label}</Label>
      {children}
    </div>
  )
}
