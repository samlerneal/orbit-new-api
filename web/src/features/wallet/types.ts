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
// ============================================================================
// Wallet Type Definitions
// ============================================================================

/**
 * Generic API response
 */
export interface ApiResponse<T = unknown> {
  success?: boolean
  message?: string
  data?: T
}

/**
 * Standard API response types
 */
export type TopupInfoResponse = ApiResponse<TopupInfo>
export type RedemptionResponse = ApiResponse<number>
export type AmountResponse = ApiResponse<string>
export type PaymentResponse = ApiResponse<Record<string, unknown>> & {
  url?: string
}
export type StripePaymentResponse = ApiResponse<{ pay_link: string }>
export type AffiliateCodeResponse = ApiResponse<string>
export type AffiliateTransferResponse = ApiResponse
export type CreemPaymentResponse = ApiResponse<{ checkout_url: string }>
export type WaffoPaymentResponse = ApiResponse<
  { payment_url?: string } | string
>
export type WaffoPancakePaymentResponse = ApiResponse<
  | {
      checkout_url?: string
      session_id?: string
      expires_at?: number | string
      order_id?: string
      // Self-service session token + expiry — surfaced by the backend so
      // future flows (refund / cancel from new-api's own UI) can use them
      // without re-issuing checkout. Not consumed by the current handler.
      token?: string
      token_expires_at?: number | string
    }
  | string
>

/**
 * Creem product configuration
 */
export interface CreemProduct {
  /** Product display name */
  name: string
  /** Creem product ID */
  productId: string
  /** Product price */
  price: number
  /** Quota amount to credit */
  quota: number
  /** Currency (USD or EUR) */
  currency: 'USD' | 'EUR'
}

/**
 * Creem payment request
 */
export interface CreemPaymentRequest {
  /** Creem product ID */
  product_id: string
  /** Payment method identifier */
  payment_method: 'creem'
}

/**
 * Payment method configuration
 */
export interface PaymentMethod {
  /** Display name of payment method */
  name: string
  /** Payment method type identifier */
  type: string
  /** Legacy optional color for UI display */
  color?: string
  /** Minimum topup amount for this payment method */
  min_topup?: number
  /** Optional react-icons component name or safe icon URL */
  icon?: string
}

/**
 * Waffo payment method configuration
 */
export interface WaffoPayMethod {
  /** Display name of payment method */
  name: string
  /** Optional icon path */
  icon?: string
  /** Waffo pay method type */
  payMethodType?: string
  /** Waffo pay method name */
  payMethodName?: string
}

/**
 * Topup configuration information
 */
export interface TopupInfo {
  /** Whether online topup is enabled */
  enable_online_topup: boolean
  /** Whether Stripe topup is enabled */
  enable_stripe_topup: boolean
  /** Available payment methods */
  pay_methods: PaymentMethod[]
  /** Minimum topup amount for online topup */
  min_topup: number
  /** Minimum topup amount for Stripe */
  stripe_min_topup: number
  /** Preset amount options */
  amount_options: number[]
  /** Discount rates by amount */
  discount: Record<number, number>
  /** Server-approved fixed topup packages */
  topup_packages: TopupPackage[]
  /** Currently visible topup campaigns */
  campaigns: TopupCampaign[]
  /** Whether promotional credit amounts are active */
  promotion_enabled: boolean
  /** Active expiring bonus quota */
  bonus_balance_quota?: number
  /** Nearest bonus expiry timestamp */
  bonus_nearest_expires_at?: number
  /** Optional topup link for purchasing codes */
  topup_link?: string
  /** Whether Creem topup is enabled */
  enable_creem_topup?: boolean
  /** Available Creem products */
  creem_products?: CreemProduct[]
  /** Whether Waffo topup is enabled */
  enable_waffo_topup?: boolean
  /** Available Waffo payment methods */
  waffo_pay_methods?: WaffoPayMethod[]
  /** Minimum topup amount for Waffo */
  waffo_min_topup?: number
  /** Whether Waffo Pancake topup is enabled */
  enable_waffo_pancake_topup?: boolean
  /** Minimum topup amount for Waffo Pancake */
  waffo_pancake_min_topup?: number
  /** Whether redemption code usage is enabled */
  enable_redemption?: boolean
  /** Whether compliance confirmation has been completed */
  payment_compliance_confirmed?: boolean
  /** Current compliance terms version */
  payment_compliance_terms_version?: string
  /** Current refund notice version required for package payments */
  refund_notice_version?: string
  /** Customer-service contact methods shown with the refund notice */
  support_contacts?: SupportContact[]
}

export type SupportContactType = 'qq' | 'wechat' | 'phone' | 'qrcode'

export interface SupportContact {
  id: string
  type: SupportContactType
  value: string
}

export const TOPUP_PACKAGE_VISUAL_STYLES = [
  'default',
  'recommended',
  'popular',
  'value',
] as const

export type TopupPackageVisualStyle =
  (typeof TOPUP_PACKAGE_VISUAL_STYLES)[number]

const TOPUP_PACKAGE_VISUAL_STYLE_CLASSES: Record<
  TopupPackageVisualStyle,
  string
> = {
  default: '',
  recommended: 'ring-1 ring-primary/20',
  popular: 'bg-primary/5 ring-2 ring-primary/30',
  value:
    'bg-amber-50/40 ring-1 ring-amber-400/30 dark:bg-amber-950/10 dark:ring-amber-700/40',
}

export function isTopupPackageVisualStyle(
  value: unknown
): value is TopupPackageVisualStyle {
  return (
    typeof value === 'string' &&
    TOPUP_PACKAGE_VISUAL_STYLES.includes(value as TopupPackageVisualStyle)
  )
}

export function getTopupPackageVisualStyleClasses(value: unknown): string {
  const style = isTopupPackageVisualStyle(value) ? value : 'default'
  return TOPUP_PACKAGE_VISUAL_STYLE_CLASSES[style]
}

export interface TopupPackage {
  /** Stable package identifier submitted to the server */
  id: string
  /** Display name */
  name: string
  /** Short usage description */
  description: string
  /** Permanent package badge */
  tag?: string
  /** Exact amount charged in RMB */
  pay_amount: number
  /** Exact amount credited in RMB */
  credit_amount: number
  /** Total credit including currently eligible campaigns */
  display_credit_amount: number
  /** Expiring campaign credit included in display credit */
  bonus_amount: number
  /** Eligible campaign badges */
  campaign_badges: Array<{
    campaign_id: string
    text: string
    valid_days: number
  }>
  /** Admin-configured selling points */
  selling_points?: string[]
  /** Admin-configured footer note */
  footer_note?: string
  /** Admin-configured visual style */
  visual_style?: TopupPackageVisualStyle
}

export interface TopupCampaign {
  id: string
  title: string
  description: string
  badge_text: string
  max_bonus: number
  valid_days: number
  priority: number
  participant_limited: boolean
  participant_total: number
  participant_remaining: number
  cumulative_max_bonus: number
}

/**
 * Preset amount option with optional discount
 */
export interface PresetAmount {
  /** Preset amount value */
  value: number
  /** Optional discount rate (0-1) */
  discount?: number
}

/**
 * Redemption code request
 */
export interface RedemptionRequest {
  /** Redemption code key */
  key: string
}

/**
 * Payment request parameters
 */
export interface PaymentRequest {
  /** Topup amount */
  amount?: number
  /** Server-approved package identifier */
  package_id?: string
  /** Payment method identifier */
  payment_method: string
  /** Whether the user accepted the current refund notice */
  refund_notice_accepted?: boolean
  /** Version of the refund notice accepted by the user */
  refund_notice_version?: string
  /** Interface language used to display the refund notice */
  refund_notice_language?: string
}

export const REFUND_NOTICE_VERSION = 'refund-notice-v1' as const

export interface RefundNoticeAcceptance {
  refund_notice_accepted: true
  refund_notice_version: typeof REFUND_NOTICE_VERSION
  refund_notice_language: string
}

export function canSubmitPackagePayment(
  refundNoticeAccepted: boolean,
  processing: boolean
): boolean {
  return refundNoticeAccepted && !processing
}

export function createPackagePaymentRequest(
  packageId: string,
  refundNotice: RefundNoticeAcceptance
): PaymentRequest {
  if (refundNotice.refund_notice_accepted !== true) {
    throw new Error('Refund notice acceptance is required')
  }

  return {
    package_id: packageId,
    payment_method: 'wxpay',
    ...refundNotice,
  }
}

/**
 * Waffo payment request parameters
 */
export interface WaffoPaymentRequest {
  /** Topup amount */
  amount: number
  /** Optional server-side Waffo payment method index */
  pay_method_index?: number
}

/**
 * Waffo Pancake payment request parameters
 */
export interface WaffoPancakePaymentRequest {
  /** Topup amount */
  amount: number
}

/**
 * Amount calculation request
 */
export interface AmountRequest {
  /** Topup amount to calculate */
  amount: number
}

/**
 * Affiliate quota transfer request
 */
export interface AffiliateTransferRequest {
  /** Quota amount to transfer */
  quota: number
}

/**
 * User wallet data
 */
export interface UserWalletData {
  /** User ID */
  id: number
  /** Username */
  username: string
  /** Current quota balance */
  quota: number
  /** Active expiring campaign quota */
  bonus_quota?: number
  /** Wallet quota plus active campaign quota */
  total_quota?: number
  /** Nearest campaign balance expiry timestamp */
  bonus_nearest_expires_at?: number
  /** Total used quota */
  used_quota: number
  /** Total request count */
  request_count: number
  /** Affiliate quota (pending rewards) */
  aff_quota: number
  /** Total affiliate quota earned (historical) */
  aff_history_quota: number
  /** Number of successful affiliate invites */
  aff_count: number
  /** User group */
  group: string
}

/**
 * Topup record status
 */
export type TopupStatus = 'success' | 'pending' | 'expired'

/**
 * Topup billing record
 */
export interface TopupRecord {
  /** Record ID */
  id: number
  /** User ID */
  user_id: number
  /** Topup amount (quota) */
  amount: number
  /** Exact raw quota credited for package-based orders */
  credit_quota?: number
  /** Expiring campaign quota credited with this order */
  bonus_credit_quota?: number
  /** Earliest expiry for campaign quota credited with this order */
  bonus_expires_at?: number
  /** Payment amount (actual money paid) */
  money: number
  /** Trade/order number */
  trade_no: string
  /** Payment method type */
  payment_method: string
  /** Creation timestamp */
  create_time: number
  /** Completion timestamp */
  complete_time?: number
  /** Payment status */
  status: TopupStatus
}

/**
 * Billing history response
 */
export interface BillingHistoryResponse {
  items: TopupRecord[]
  total: number
}

/**
 * Complete order request (admin only)
 */
export interface CompleteOrderRequest {
  trade_no: string
}
