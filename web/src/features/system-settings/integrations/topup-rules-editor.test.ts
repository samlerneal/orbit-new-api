import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  mergeCampaignPatchForEdit,
  normalizeCampaign,
  type CampaignRule,
  type TopupPackageRule,
} from './topup-rules-editor'

const packages: TopupPackageRule[] = [
  {
    id: 'advanced',
    name: 'Advanced',
    description: '',
    tag: '',
    pay_amount: 98,
    credit_amount: 100,
    enabled: true,
    sort_order: 1,
    selling_points: [],
    footer_note: '',
    visual_style: 'default',
  },
  {
    id: 'professional',
    name: 'Professional',
    description: '',
    tag: '',
    pay_amount: 490,
    credit_amount: 500,
    enabled: true,
    sort_order: 2,
    selling_points: [],
    footer_note: '',
    visual_style: 'default',
  },
]

function legacyCampaign(): CampaignRule {
  return {
    id: 'launch-first-topup-30',
    name: 'Launch first top-up',
    banner_title: 'First top-up bonus: 30%',
    banner_text:
      'Complete your first top-up in this campaign to receive time-limited bonus balance.',
    badge_text: 'First top-up +30%',
    enabled: false,
    starts_at: 0,
    ends_at: 0,
    package_ids: ['advanced', 'professional'],
    eligibility: 'per_campaign',
    max_claims_per_user: 1,
    max_claims_per_email: 1,
    max_claims_total: 100,
    max_participants_total: 0,
    reservation_minutes: 3,
    reward_mode: 'target_total_percent',
    reward_percent: 30,
    fixed_bonus: {},
    rounding_mode: 'ceil_yuan',
    valid_days: 45,
    stackable: false,
    priority: 100,
  }
}

describe('campaign legacy edit path', () => {
  test('direct per_campaign edit carries migration version into first save object', () => {
    const oldReadback = legacyCampaign()
    const edited = mergeCampaignPatchForEdit(oldReadback, {
      eligibility: 'per_campaign',
      max_claims_per_user: 2,
      max_claims_per_email: 2,
    })
    const firstSave = normalizeCampaign(edited, packages)

    assert.equal(firstSave.eligibility, 'per_campaign')
    assert.equal(firstSave.legacy_migration_version, 1)
    assert.equal(firstSave.max_claims_total, 0)
  })
})
