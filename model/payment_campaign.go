package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	BonusBalanceStatusActive  = "active"
	BonusBalanceStatusExpired = "expired"
)

// PaymentCampaignClaim is the durable eligibility ledger for a campaign.
// ClaimKey is nullable so unlimited campaigns can create multiple claims while
// limited rules use a database unique constraint for race-safe enforcement.
type PaymentCampaignClaim struct {
	Id         int     `json:"id"`
	CampaignId string  `json:"campaign_id" gorm:"type:varchar(64);index:idx_campaign_claim_user,priority:1;uniqueIndex:idx_campaign_topup,priority:1"`
	UserId     int     `json:"user_id" gorm:"index:idx_campaign_claim_user,priority:2"`
	PackageId  string  `json:"package_id" gorm:"type:varchar(64);index"`
	TopUpId    int     `json:"topup_id" gorm:"uniqueIndex:idx_campaign_topup,priority:2"`
	ClaimKey   *string `json:"claim_key,omitempty" gorm:"type:varchar(255);uniqueIndex"`
	CreatedAt  int64   `json:"created_at" gorm:"bigint;index"`
}

// BonusBalance is an expiring balance lot. Separate lots preserve independent
// expiry dates across overlapping campaigns.
type BonusBalance struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index:idx_bonus_user_expiry,priority:1"`
	CampaignId  string `json:"campaign_id" gorm:"type:varchar(64);index;uniqueIndex:idx_bonus_campaign_topup,priority:1"`
	TopUpId     int    `json:"topup_id" gorm:"uniqueIndex:idx_bonus_campaign_topup,priority:2"`
	AmountTotal int64  `json:"amount_total" gorm:"type:bigint;not null"`
	AmountUsed  int64  `json:"amount_used" gorm:"type:bigint;not null;default:0"`
	ExpiresAt   int64  `json:"expires_at" gorm:"type:bigint;index:idx_bonus_user_expiry,priority:2"`
	Status      string `json:"status" gorm:"type:varchar(24);index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint"`
}

func (balance *BonusBalance) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	balance.CreatedAt = now
	balance.UpdatedAt = now
	if balance.Status == "" {
		balance.Status = BonusBalanceStatusActive
	}
	return nil
}

func (balance *BonusBalance) BeforeUpdate(tx *gorm.DB) error {
	balance.UpdatedAt = common.GetTimestamp()
	return nil
}

type CampaignAwardSnapshot struct {
	CampaignId     string `json:"campaign_id"`
	CampaignName   string `json:"campaign_name"`
	Eligibility    string `json:"eligibility"`
	MaxClaims      int    `json:"max_claims"`
	PackageId      string `json:"package_id"`
	BonusQuota     int64  `json:"bonus_quota"`
	BonusAmountCNY string `json:"bonus_amount_cny"`
	ValidDays      int    `json:"valid_days"`
}

type CampaignOffer struct {
	Campaign    operation_setting.PaymentCampaign `json:"campaign"`
	BonusAmount float64                           `json:"bonus_amount"`
	TotalAmount float64                           `json:"total_amount"`
	BonusQuota  int64                             `json:"bonus_quota"`
}

func campaignAppliesToPackage(campaign operation_setting.PaymentCampaign, packageId string) bool {
	if len(campaign.PackageIDs) == 0 {
		return true
	}
	for _, candidate := range campaign.PackageIDs {
		if candidate == packageId {
			return true
		}
	}
	return false
}

func campaignIsActive(campaign operation_setting.PaymentCampaign, now int64) bool {
	if !campaign.Enabled {
		return false
	}
	if campaign.StartsAt > 0 && now < campaign.StartsAt {
		return false
	}
	return campaign.EndsAt <= 0 || now < campaign.EndsAt
}

func campaignClaimCount(userId int, campaign operation_setting.PaymentCampaign, packageId string) (int64, error) {
	query := DB.Model(&PaymentCampaignClaim{}).
		Where("user_id = ? AND campaign_id = ?", userId, campaign.ID)
	if campaign.Eligibility == operation_setting.CampaignEligibilityPerPackage {
		query = query.Where("package_id = ?", packageId)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func campaignUserIsEligible(userId int, campaign operation_setting.PaymentCampaign, packageId string) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid user id")
	}
	if campaign.Eligibility == operation_setting.CampaignEligibilityUnlimited && campaign.MaxClaimsPerUser == 0 {
		return true, nil
	}
	count, err := campaignClaimCount(userId, campaign, packageId)
	if err != nil {
		return false, err
	}
	limit := campaign.MaxClaimsPerUser
	if limit <= 0 {
		limit = 1
	}
	return count < int64(limit), nil
}

func quotaFromCNY(amount decimal.Decimal) (int64, error) {
	exchangeRate := decimal.NewFromFloat(operation_setting.USDExchangeRate)
	if amount.LessThanOrEqual(decimal.Zero) || exchangeRate.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("invalid amount or exchange rate")
	}
	quota, clamp := common.QuotaFromDecimalChecked(
		amount.Div(exchangeRate).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if clamp != nil || quota <= 0 {
		return 0, errors.New("campaign quota is outside the supported range")
	}
	return int64(quota), nil
}

func resolveCampaignReward(campaign operation_setting.PaymentCampaign, packageOption operation_setting.TopupPackage) (decimal.Decimal, decimal.Decimal, error) {
	payAmount := decimal.NewFromFloat(packageOption.PayAmount)
	regularAmount := decimal.NewFromFloat(packageOption.CreditAmount)
	var totalAmount decimal.Decimal

	switch campaign.RewardMode {
	case operation_setting.CampaignRewardTargetTotalPercent:
		multiplier := decimal.NewFromInt(100).
			Add(decimal.NewFromFloat(campaign.RewardPercent)).
			Div(decimal.NewFromInt(100))
		totalAmount = payAmount.Mul(multiplier)
	case operation_setting.CampaignRewardFixedBonus:
		fixedAmount := decimal.NewFromFloat(campaign.FixedBonus[packageOption.ID])
		totalAmount = regularAmount.Add(fixedAmount)
	default:
		return decimal.Zero, decimal.Zero, errors.New("unsupported campaign reward mode")
	}

	if campaign.RoundingMode == operation_setting.CampaignRoundingCeilYuan {
		totalAmount = totalAmount.Ceil()
	}
	bonusAmount := totalAmount.Sub(regularAmount)
	if bonusAmount.LessThanOrEqual(decimal.Zero) {
		return totalAmount, decimal.Zero, nil
	}
	return totalAmount, bonusAmount, nil
}

func ResolveCampaignOffers(userId int, packageOption operation_setting.TopupPackage, now int64) ([]CampaignOffer, error) {
	campaigns := operation_setting.GetPaymentCampaigns()
	sort.SliceStable(campaigns, func(i, j int) bool {
		return campaigns[i].Priority > campaigns[j].Priority
	})

	offers := make([]CampaignOffer, 0)
	for _, campaign := range campaigns {
		if !campaignIsActive(campaign, now) || !campaignAppliesToPackage(campaign, packageOption.ID) {
			continue
		}
		eligible, err := campaignUserIsEligible(userId, campaign, packageOption.ID)
		if err != nil {
			return nil, err
		}
		if !eligible {
			continue
		}
		totalAmount, bonusAmount, err := resolveCampaignReward(campaign, packageOption)
		if err != nil {
			return nil, err
		}
		if bonusAmount.LessThanOrEqual(decimal.Zero) {
			continue
		}
		bonusQuota, err := quotaFromCNY(bonusAmount)
		if err != nil {
			return nil, err
		}
		offers = append(offers, CampaignOffer{
			Campaign:    campaign,
			BonusAmount: bonusAmount.InexactFloat64(),
			TotalAmount: totalAmount.InexactFloat64(),
			BonusQuota:  bonusQuota,
		})
		if !campaign.Stackable {
			break
		}
	}
	return offers, nil
}

func BuildCampaignAwardSnapshots(userId int, packageOption operation_setting.TopupPackage, now int64) ([]CampaignAwardSnapshot, error) {
	offers, err := ResolveCampaignOffers(userId, packageOption, now)
	if err != nil {
		return nil, err
	}
	snapshots := make([]CampaignAwardSnapshot, 0, len(offers))
	for _, offer := range offers {
		snapshots = append(snapshots, CampaignAwardSnapshot{
			CampaignId:     offer.Campaign.ID,
			CampaignName:   offer.Campaign.Name,
			Eligibility:    offer.Campaign.Eligibility,
			MaxClaims:      offer.Campaign.MaxClaimsPerUser,
			PackageId:      packageOption.ID,
			BonusQuota:     offer.BonusQuota,
			BonusAmountCNY: decimal.NewFromFloat(offer.BonusAmount).StringFixed(2),
			ValidDays:      offer.Campaign.ValidDays,
		})
	}
	return snapshots, nil
}

func EncodeCampaignAwardSnapshots(snapshots []CampaignAwardSnapshot) string {
	if len(snapshots) == 0 {
		return ""
	}
	encoded, err := json.Marshal(snapshots)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func DecodeCampaignAwardSnapshots(raw string) ([]CampaignAwardSnapshot, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var snapshots []CampaignAwardSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshots); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func campaignClaimKey(snapshot CampaignAwardSnapshot, userId int, slot int64) *string {
	var value string
	switch snapshot.Eligibility {
	case operation_setting.CampaignEligibilityPerCampaign:
		value = fmt.Sprintf("%s:user:%d:slot:%d", snapshot.CampaignId, userId, slot)
	case operation_setting.CampaignEligibilityPerPackage:
		value = fmt.Sprintf("%s:user:%d:package:%s:slot:%d", snapshot.CampaignId, userId, snapshot.PackageId, slot)
	default:
		return nil
	}
	return &value
}

func applyCampaignAwardsTx(tx *gorm.DB, topUp *TopUp) (int64, error) {
	snapshots, err := DecodeCampaignAwardSnapshots(topUp.CampaignSnapshot)
	if err != nil || len(snapshots) == 0 {
		return 0, err
	}

	if err := lockForUpdate(tx).Select("id").Where("id = ?", topUp.UserId).First(&User{}).Error; err != nil {
		return 0, err
	}

	var awarded int64
	for _, snapshot := range snapshots {
		if snapshot.BonusQuota <= 0 || snapshot.ValidDays <= 0 {
			continue
		}
		maxClaims := snapshot.MaxClaims
		if maxClaims <= 0 && snapshot.Eligibility != operation_setting.CampaignEligibilityUnlimited {
			maxClaims = 1
		}
		claimCreated := false
		attemptLimit := maxClaims + 1
		if attemptLimit <= 1 {
			attemptLimit = 2
		}
		for attempt := 0; attempt < attemptLimit; attempt++ {
			query := tx.Model(&PaymentCampaignClaim{}).
				Where("campaign_id = ? AND user_id = ?", snapshot.CampaignId, topUp.UserId)
			if snapshot.Eligibility == operation_setting.CampaignEligibilityPerPackage {
				query = query.Where("package_id = ?", snapshot.PackageId)
			}
			var count int64
			if err := query.Count(&count).Error; err != nil {
				return awarded, err
			}
			if maxClaims > 0 && count >= int64(maxClaims) {
				break
			}
			claim := &PaymentCampaignClaim{
				CampaignId: snapshot.CampaignId,
				UserId:     topUp.UserId,
				PackageId:  snapshot.PackageId,
				TopUpId:    topUp.Id,
				ClaimKey:   campaignClaimKey(snapshot, topUp.UserId, count+1),
				CreatedAt:  common.GetTimestamp(),
			}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(claim)
			if result.Error != nil {
				return awarded, result.Error
			}
			if result.RowsAffected == 1 {
				claimCreated = true
				break
			}
			var existing int64
			if err := tx.Model(&PaymentCampaignClaim{}).
				Where("campaign_id = ? AND top_up_id = ?", snapshot.CampaignId, topUp.Id).
				Count(&existing).Error; err != nil {
				return awarded, err
			}
			if existing > 0 {
				break
			}
		}
		if !claimCreated {
			continue
		}
		balance := &BonusBalance{
			UserId:      topUp.UserId,
			CampaignId:  snapshot.CampaignId,
			TopUpId:     topUp.Id,
			AmountTotal: snapshot.BonusQuota,
			ExpiresAt:   common.GetTimestamp() + int64(snapshot.ValidDays)*24*60*60,
		}
		if err := tx.Create(balance).Error; err != nil {
			return awarded, err
		}
		awarded += snapshot.BonusQuota
	}
	return awarded, nil
}

type BonusBalanceSummary struct {
	ActiveQuota      int64 `json:"active_quota"`
	NearestExpiresAt int64 `json:"nearest_expires_at"`
}

func GetBonusBalanceSummary(userId int) (BonusBalanceSummary, error) {
	now := common.GetTimestamp()
	var balances []BonusBalance
	if err := DB.Where("user_id = ? AND status = ? AND expires_at > ? AND amount_total > amount_used", userId, BonusBalanceStatusActive, now).
		Order("expires_at asc, id asc").Find(&balances).Error; err != nil {
		return BonusBalanceSummary{}, err
	}
	summary := BonusBalanceSummary{}
	for _, balance := range balances {
		summary.ActiveQuota += balance.AmountTotal - balance.AmountUsed
		if summary.NearestExpiresAt == 0 || balance.ExpiresAt < summary.NearestExpiresAt {
			summary.NearestExpiresAt = balance.ExpiresAt
		}
	}
	return summary, nil
}
