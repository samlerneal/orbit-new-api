package model

import (
	"crypto/sha256"
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

	CampaignClaimStatusReserved = "reserved"
	CampaignClaimStatusAwarded  = "awarded"
	CampaignClaimStatusReleased = "released"
)

var ErrCampaignReservationUnavailable = errors.New("campaign reservation unavailable")

// PaymentCampaignState provides one durable serialization row per campaign.
// Incrementing Revision acquires a database write lock before capacity checks.
type PaymentCampaignState struct {
	CampaignId string `json:"campaign_id" gorm:"type:varchar(64);primaryKey"`
	Revision   int64  `json:"revision" gorm:"type:bigint;not null;default:0"`
	UpdatedAt  int64  `json:"updated_at" gorm:"type:bigint;not null;default:0"`
}

// PaymentCampaignClaim is both the short-lived reservation and the durable
// eligibility ledger. Released rows retain audit metadata while nullable unique
// keys are cleared so their account, email and campaign slots can be reused.
type PaymentCampaignClaim struct {
	Id              int     `json:"id"`
	CampaignId      string  `json:"campaign_id" gorm:"type:varchar(64);index:idx_campaign_claim_user,priority:1;uniqueIndex:idx_campaign_topup,priority:1;index:idx_campaign_claim_status_expiry,priority:1"`
	UserId          int     `json:"user_id" gorm:"index:idx_campaign_claim_user,priority:2"`
	PackageId       string  `json:"package_id" gorm:"type:varchar(64);index"`
	TopUpId         int     `json:"topup_id" gorm:"column:topup_id;uniqueIndex:idx_campaign_topup,priority:2"`
	Status          string  `json:"status" gorm:"type:varchar(24);not null;default:'awarded';index:idx_campaign_claim_status_expiry,priority:2"`
	EmailHash       string  `json:"-" gorm:"type:char(64);index"`
	ClaimKey        *string `json:"-" gorm:"type:varchar(255);uniqueIndex"`
	EmailClaimKey   *string `json:"-" gorm:"type:varchar(255)"`
	CampaignSlotKey *string `json:"-" gorm:"type:varchar(160)"`
	ReservedUntil   int64   `json:"reserved_until" gorm:"type:bigint;index:idx_campaign_claim_status_expiry,priority:3"`
	AwardedAt       int64   `json:"awarded_at" gorm:"type:bigint"`
	ReleasedAt      int64   `json:"released_at" gorm:"type:bigint"`
	CreatedAt       int64   `json:"created_at" gorm:"type:bigint;index"`
	UpdatedAt       int64   `json:"updated_at" gorm:"type:bigint"`
}

func (claim *PaymentCampaignClaim) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if claim.CreatedAt == 0 {
		claim.CreatedAt = now
	}
	claim.UpdatedAt = now
	if claim.Status == "" {
		claim.Status = CampaignClaimStatusAwarded
	}
	return nil
}

func ensurePaymentCampaignClaimIndexes(db *gorm.DB) error {
	indexes := []struct {
		name   string
		column string
	}{
		{name: "idx_campaign_email_claim_key", column: "email_claim_key"},
		{name: "idx_campaign_slot_key", column: "campaign_slot_key"},
	}
	for _, index := range indexes {
		if db.Migrator().HasIndex(&PaymentCampaignClaim{}, index.name) {
			continue
		}
		statement := fmt.Sprintf(
			"CREATE UNIQUE INDEX %s ON payment_campaign_claims (%s)",
			index.name,
			index.column,
		)
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

// BonusBalance is an expiring balance lot. Separate lots preserve independent
// expiry dates across overlapping campaigns.
type BonusBalance struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index:idx_bonus_user_expiry,priority:1"`
	CampaignId  string `json:"campaign_id" gorm:"type:varchar(64);index;uniqueIndex:idx_bonus_campaign_topup,priority:1"`
	TopUpId     int    `json:"topup_id" gorm:"column:topup_id;uniqueIndex:idx_bonus_campaign_topup,priority:2"`
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
	CampaignId         string `json:"campaign_id"`
	CampaignName       string `json:"campaign_name"`
	Eligibility        string `json:"eligibility"`
	MaxClaims          int    `json:"max_claims"`
	MaxClaimsPerEmail  int    `json:"max_claims_per_email"`
	MaxClaimsTotal     int    `json:"max_claims_total"`
	ReservationMinutes int    `json:"reservation_minutes"`
	PackageId          string `json:"package_id"`
	BonusQuota         int64  `json:"bonus_quota"`
	BonusAmountCNY     string `json:"bonus_amount_cny"`
	ValidDays          int    `json:"valid_days"`
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

func campaignActiveClaimQuery(db *gorm.DB, campaignId string, now int64) *gorm.DB {
	return db.Model(&PaymentCampaignClaim{}).
		Where("campaign_id = ?", campaignId).
		Where(
			"status = ? OR status = '' OR (status = ? AND reserved_until > ?)",
			CampaignClaimStatusAwarded,
			CampaignClaimStatusReserved,
			now,
		)
}

func campaignClaimCount(db *gorm.DB, userId int, campaign operation_setting.PaymentCampaign, packageId string, now int64) (int64, error) {
	query := campaignActiveClaimQuery(db, campaign.ID, now).
		Where("user_id = ?", userId)
	if campaign.Eligibility == operation_setting.CampaignEligibilityPerPackage {
		query = query.Where("package_id = ?", packageId)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func CampaignEmailHash(email string) string {
	normalized := NormalizeEmail(email)
	if normalized == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
}

func campaignUserIsEligible(userId int, campaign operation_setting.PaymentCampaign, packageId string, now int64) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid user id")
	}
	if campaign.MaxClaimsTotal > 0 {
		var total int64
		if err := campaignActiveClaimQuery(DB, campaign.ID, now).Count(&total).Error; err != nil {
			return false, err
		}
		if total >= int64(campaign.MaxClaimsTotal) {
			return false, nil
		}
	}

	if campaign.Eligibility != operation_setting.CampaignEligibilityUnlimited || campaign.MaxClaimsPerUser > 0 {
		count, err := campaignClaimCount(DB, userId, campaign, packageId, now)
		if err != nil {
			return false, err
		}
		limit := campaign.MaxClaimsPerUser
		if limit <= 0 {
			limit = 1
		}
		if count >= int64(limit) {
			return false, nil
		}
	}

	if campaign.MaxClaimsPerEmail > 0 {
		var user User
		if err := DB.Select("email").First(&user, userId).Error; err != nil {
			return false, err
		}
		emailHash := CampaignEmailHash(user.Email)
		if emailHash == "" {
			return false, nil
		}
		var count int64
		if err := campaignActiveClaimQuery(DB, campaign.ID, now).
			Where("email_hash = ?", emailHash).
			Count(&count).Error; err != nil {
			return false, err
		}
		if count >= int64(campaign.MaxClaimsPerEmail) {
			return false, nil
		}
	}
	return true, nil
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
		eligible, err := campaignUserIsEligible(userId, campaign, packageOption.ID, now)
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
			CampaignId:         offer.Campaign.ID,
			CampaignName:       offer.Campaign.Name,
			Eligibility:        offer.Campaign.Eligibility,
			MaxClaims:          offer.Campaign.MaxClaimsPerUser,
			MaxClaimsPerEmail:  offer.Campaign.MaxClaimsPerEmail,
			MaxClaimsTotal:     offer.Campaign.MaxClaimsTotal,
			ReservationMinutes: offer.Campaign.ReservationMinutes,
			PackageId:          packageOption.ID,
			BonusQuota:         offer.BonusQuota,
			BonusAmountCNY:     decimal.NewFromFloat(offer.BonusAmount).StringFixed(2),
			ValidDays:          offer.Campaign.ValidDays,
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

func findPaymentCampaign(campaignId string) (operation_setting.PaymentCampaign, bool) {
	for _, campaign := range operation_setting.GetPaymentCampaigns() {
		if campaign.ID == campaignId {
			return campaign, true
		}
	}
	return operation_setting.PaymentCampaign{}, false
}

func lockCampaignStateTx(tx *gorm.DB, campaignId string, now int64) error {
	state := &PaymentCampaignState{CampaignId: campaignId, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(state).Error; err != nil {
		return err
	}
	result := tx.Model(&PaymentCampaignState{}).
		Where("campaign_id = ?", campaignId).
		Updates(map[string]interface{}{
			"revision":   gorm.Expr("revision + 1"),
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("failed to lock campaign state")
	}
	return nil
}

func releaseExpiredCampaignReservationsLockedTx(tx *gorm.DB, campaignId string, now int64) error {
	return tx.Model(&PaymentCampaignClaim{}).
		Where("campaign_id = ? AND status = ? AND reserved_until <= ?", campaignId, CampaignClaimStatusReserved, now).
		Updates(map[string]interface{}{
			"status":            CampaignClaimStatusReleased,
			"claim_key":         nil,
			"email_claim_key":   nil,
			"campaign_slot_key": nil,
			"released_at":       now,
			"updated_at":        now,
		}).Error
}

func firstAvailableCampaignKeyTx(
	tx *gorm.DB,
	campaignId string,
	column string,
	limit int,
	now int64,
	build func(int) string,
) (*string, error) {
	if limit <= 0 {
		return nil, nil
	}
	var used []string
	if err := campaignActiveClaimQuery(tx, campaignId, now).
		Where(column+" IS NOT NULL").
		Pluck(column, &used).Error; err != nil {
		return nil, err
	}
	usedSet := make(map[string]struct{}, len(used))
	for _, key := range used {
		usedSet[key] = struct{}{}
	}
	for slot := 1; slot <= limit; slot++ {
		key := build(slot)
		if _, exists := usedSet[key]; !exists {
			return &key, nil
		}
	}
	return nil, nil
}

func campaignUserClaimPrefix(campaign operation_setting.PaymentCampaign, topUp *TopUp) string {
	if campaign.Eligibility == operation_setting.CampaignEligibilityPerPackage {
		return fmt.Sprintf("%s:user:%d:package:%s", campaign.ID, topUp.UserId, topUp.PackageId)
	}
	return fmt.Sprintf("%s:user:%d", campaign.ID, topUp.UserId)
}

type campaignReservationKeys struct {
	user         *string
	email        *string
	campaignSlot *string
}

func buildCampaignReservationKeysTx(
	tx *gorm.DB,
	topUp *TopUp,
	campaign operation_setting.PaymentCampaign,
	emailHash string,
	now int64,
) (campaignReservationKeys, bool, error) {
	userLimit := campaign.MaxClaimsPerUser
	if campaign.Eligibility != operation_setting.CampaignEligibilityUnlimited && userLimit <= 0 {
		userLimit = 1
	}
	if userLimit > 0 {
		count, err := campaignClaimCount(tx, topUp.UserId, campaign, topUp.PackageId, now)
		if err != nil || count >= int64(userLimit) {
			return campaignReservationKeys{}, false, err
		}
	}
	userPrefix := campaignUserClaimPrefix(campaign, topUp)
	userKey, err := firstAvailableCampaignKeyTx(
		tx, campaign.ID, "claim_key", userLimit, now,
		func(slot int) string { return fmt.Sprintf("%s:slot:%d", userPrefix, slot) },
	)
	if err != nil || (userLimit > 0 && userKey == nil) {
		return campaignReservationKeys{}, false, err
	}

	if campaign.MaxClaimsPerEmail > 0 && emailHash == "" {
		return campaignReservationKeys{}, false, nil
	}
	if campaign.MaxClaimsPerEmail > 0 {
		var count int64
		err := campaignActiveClaimQuery(tx, campaign.ID, now).
			Where("email_hash = ?", emailHash).
			Count(&count).Error
		if err != nil || count >= int64(campaign.MaxClaimsPerEmail) {
			return campaignReservationKeys{}, false, err
		}
	}
	emailKey, err := firstAvailableCampaignKeyTx(
		tx, campaign.ID, "email_claim_key", campaign.MaxClaimsPerEmail, now,
		func(slot int) string {
			return fmt.Sprintf("%s:email:%s:slot:%d", campaign.ID, emailHash, slot)
		},
	)
	if err != nil || (campaign.MaxClaimsPerEmail > 0 && emailKey == nil) {
		return campaignReservationKeys{}, false, err
	}

	if campaign.MaxClaimsTotal > 0 {
		var count int64
		err := campaignActiveClaimQuery(tx, campaign.ID, now).Count(&count).Error
		if err != nil || count >= int64(campaign.MaxClaimsTotal) {
			return campaignReservationKeys{}, false, err
		}
	}
	campaignSlotKey, err := firstAvailableCampaignKeyTx(
		tx, campaign.ID, "campaign_slot_key", campaign.MaxClaimsTotal, now,
		func(slot int) string { return fmt.Sprintf("%s:campaign-slot:%d", campaign.ID, slot) },
	)
	if err != nil || (campaign.MaxClaimsTotal > 0 && campaignSlotKey == nil) {
		return campaignReservationKeys{}, false, err
	}
	return campaignReservationKeys{
		user: userKey, email: emailKey, campaignSlot: campaignSlotKey,
	}, true, nil
}

func saveCampaignReservationTx(
	tx *gorm.DB,
	existing *PaymentCampaignClaim,
	topUp *TopUp,
	campaign operation_setting.PaymentCampaign,
	emailHash string,
	keys campaignReservationKeys,
	now int64,
) (*PaymentCampaignClaim, error) {
	reservationMinutes := campaign.ReservationMinutes
	if reservationMinutes <= 0 {
		reservationMinutes = operation_setting.DefaultCampaignReservationMinutes
	}
	reservedUntil := now + int64(reservationMinutes)*60
	if existing.Id == 0 {
		claim := &PaymentCampaignClaim{
			CampaignId: campaign.ID, UserId: topUp.UserId, PackageId: topUp.PackageId,
			TopUpId: topUp.Id, Status: CampaignClaimStatusReserved, EmailHash: emailHash,
			ClaimKey: keys.user, EmailClaimKey: keys.email, CampaignSlotKey: keys.campaignSlot,
			ReservedUntil: reservedUntil, CreatedAt: now, UpdatedAt: now,
		}
		return claim, tx.Create(claim).Error
	}
	values := map[string]interface{}{
		"status": CampaignClaimStatusReserved, "email_hash": emailHash,
		"claim_key": keys.user, "email_claim_key": keys.email,
		"campaign_slot_key": keys.campaignSlot, "reserved_until": reservedUntil,
		"awarded_at": 0, "released_at": 0, "updated_at": now,
	}
	if err := tx.Model(existing).Updates(values).Error; err != nil {
		return nil, err
	}
	return existing, tx.First(existing, existing.Id).Error
}

func reserveCampaignClaimLockedTx(
	tx *gorm.DB,
	topUp *TopUp,
	campaign operation_setting.PaymentCampaign,
	emailHash string,
	now int64,
) (*PaymentCampaignClaim, bool, error) {
	var existing PaymentCampaignClaim
	err := tx.Where("campaign_id = ? AND topup_id = ?", campaign.ID, topUp.Id).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if err == nil {
		if existing.Status == CampaignClaimStatusAwarded || existing.Status == "" {
			return &existing, false, nil
		}
		if existing.Status == CampaignClaimStatusReserved && existing.ReservedUntil > now {
			return &existing, true, nil
		}
	}

	keys, available, err := buildCampaignReservationKeysTx(tx, topUp, campaign, emailHash, now)
	if err != nil || !available {
		return nil, false, err
	}
	claim, err := saveCampaignReservationTx(tx, &existing, topUp, campaign, emailHash, keys, now)
	return claim, err == nil, err
}

func syncSnapshotSafeguards(snapshot CampaignAwardSnapshot, campaign operation_setting.PaymentCampaign) CampaignAwardSnapshot {
	snapshot.Eligibility = campaign.Eligibility
	snapshot.MaxClaims = campaign.MaxClaimsPerUser
	snapshot.MaxClaimsPerEmail = campaign.MaxClaimsPerEmail
	snapshot.MaxClaimsTotal = campaign.MaxClaimsTotal
	snapshot.ReservationMinutes = campaign.ReservationMinutes
	return snapshot
}

func CreateTopUpWithCampaignReservations(
	topUp *TopUp,
	snapshots []CampaignAwardSnapshot,
	now int64,
) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("id", "email").First(&user, topUp.UserId).Error; err != nil {
			return err
		}
		emailHash := CampaignEmailHash(user.Email)
		topUp.CampaignSnapshot = ""
		if err := tx.Create(topUp).Error; err != nil {
			return err
		}

		sort.SliceStable(snapshots, func(i, j int) bool {
			return snapshots[i].CampaignId < snapshots[j].CampaignId
		})
		reserved := make([]CampaignAwardSnapshot, 0, len(snapshots))
		for _, snapshot := range snapshots {
			campaign, exists := findPaymentCampaign(snapshot.CampaignId)
			if !exists ||
				!campaignIsActive(campaign, now) ||
				!campaignAppliesToPackage(campaign, topUp.PackageId) {
				continue
			}
			if err := lockCampaignStateTx(tx, campaign.ID, now); err != nil {
				return err
			}
			if err := releaseExpiredCampaignReservationsLockedTx(tx, campaign.ID, now); err != nil {
				return err
			}
			snapshot = syncSnapshotSafeguards(snapshot, campaign)
			_, didReserve, err := reserveCampaignClaimLockedTx(tx, topUp, campaign, emailHash, now)
			if err != nil {
				return err
			}
			if didReserve {
				reserved = append(reserved, snapshot)
			}
		}
		if len(snapshots) > 0 && len(reserved) == 0 {
			return ErrCampaignReservationUnavailable
		}
		topUp.CampaignSnapshot = EncodeCampaignAwardSnapshots(reserved)
		return tx.Model(topUp).Update("campaign_snapshot", topUp.CampaignSnapshot).Error
	})
}

func legacyCampaignFromSnapshot(snapshot CampaignAwardSnapshot) operation_setting.PaymentCampaign {
	return operation_setting.PaymentCampaign{
		ID:                 snapshot.CampaignId,
		Enabled:            true,
		Eligibility:        snapshot.Eligibility,
		MaxClaimsPerUser:   snapshot.MaxClaims,
		MaxClaimsPerEmail:  snapshot.MaxClaimsPerEmail,
		MaxClaimsTotal:     snapshot.MaxClaimsTotal,
		ReservationMinutes: operation_setting.DefaultCampaignReservationMinutes,
	}
}

func ensureCampaignClaimForAwardTx(
	tx *gorm.DB,
	topUp *TopUp,
	snapshot CampaignAwardSnapshot,
	emailHash string,
	now int64,
) (*PaymentCampaignClaim, bool, error) {
	if err := lockCampaignStateTx(tx, snapshot.CampaignId, now); err != nil {
		return nil, false, err
	}
	if err := releaseExpiredCampaignReservationsLockedTx(tx, snapshot.CampaignId, now); err != nil {
		return nil, false, err
	}

	var claim PaymentCampaignClaim
	err := tx.Where("campaign_id = ? AND topup_id = ?", snapshot.CampaignId, topUp.Id).First(&claim).Error
	if err == nil {
		if claim.Status == CampaignClaimStatusAwarded || claim.Status == "" {
			return &claim, false, nil
		}
		if claim.Status == CampaignClaimStatusReserved && claim.ReservedUntil > now {
			return &claim, true, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	campaign, exists := findPaymentCampaign(snapshot.CampaignId)
	if !exists || !campaignIsActive(campaign, now) || !campaignAppliesToPackage(campaign, topUp.PackageId) {
		if snapshot.MaxClaimsTotal == 0 && snapshot.ReservationMinutes == 0 && errors.Is(err, gorm.ErrRecordNotFound) {
			campaign = legacyCampaignFromSnapshot(snapshot)
		} else {
			return nil, false, nil
		}
	}
	return reserveCampaignClaimLockedTx(tx, topUp, campaign, emailHash, now)
}

func applyCampaignAwardsTx(tx *gorm.DB, topUp *TopUp) (int64, error) {
	snapshots, err := DecodeCampaignAwardSnapshots(topUp.CampaignSnapshot)
	if err != nil || len(snapshots) == 0 {
		return 0, err
	}

	var user User
	if err := lockForUpdate(tx).Select("id", "email").Where("id = ?", topUp.UserId).First(&user).Error; err != nil {
		return 0, err
	}
	emailHash := CampaignEmailHash(user.Email)
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].CampaignId < snapshots[j].CampaignId
	})

	var awarded int64
	now := common.GetTimestamp()
	for _, snapshot := range snapshots {
		if snapshot.BonusQuota <= 0 || snapshot.ValidDays <= 0 {
			continue
		}
		claim, canAward, err := ensureCampaignClaimForAwardTx(tx, topUp, snapshot, emailHash, now)
		if err != nil {
			return awarded, err
		}
		if !canAward || claim == nil {
			continue
		}
		result := tx.Model(&PaymentCampaignClaim{}).
			Where("id = ? AND status = ?", claim.Id, CampaignClaimStatusReserved).
			Updates(map[string]interface{}{
				"status":         CampaignClaimStatusAwarded,
				"awarded_at":     now,
				"reserved_until": 0,
				"updated_at":     now,
			})
		if result.Error != nil {
			return awarded, result.Error
		}
		if result.RowsAffected != 1 {
			continue
		}
		balance := &BonusBalance{
			UserId:      topUp.UserId,
			CampaignId:  snapshot.CampaignId,
			TopUpId:     topUp.Id,
			AmountTotal: snapshot.BonusQuota,
			ExpiresAt:   now + int64(snapshot.ValidDays)*24*60*60,
		}
		if err := tx.Create(balance).Error; err != nil {
			return awarded, err
		}
		awarded += snapshot.BonusQuota
	}
	return awarded, nil
}

func releaseTopUpCampaignReservationsTx(tx *gorm.DB, topUpId int, now int64) error {
	var claims []PaymentCampaignClaim
	if err := tx.Where("topup_id = ? AND status = ?", topUpId, CampaignClaimStatusReserved).
		Order("campaign_id asc").
		Find(&claims).Error; err != nil {
		return err
	}
	for _, claim := range claims {
		if err := lockCampaignStateTx(tx, claim.CampaignId, now); err != nil {
			return err
		}
		if err := tx.Model(&claim).Updates(map[string]interface{}{
			"status":            CampaignClaimStatusReleased,
			"claim_key":         nil,
			"email_claim_key":   nil,
			"campaign_slot_key": nil,
			"released_at":       now,
			"updated_at":        now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

type PaymentCampaignClaimStats struct {
	CampaignId string `json:"campaign_id"`
	Limited    bool   `json:"limited"`
	Total      int64  `json:"total"`
	Awarded    int64  `json:"awarded"`
	Reserved   int64  `json:"reserved"`
	Remaining  int64  `json:"remaining"`
}

func GetPaymentCampaignClaimStats(now int64) ([]PaymentCampaignClaimStats, error) {
	campaigns := operation_setting.GetPaymentCampaigns()
	stats := make([]PaymentCampaignClaimStats, 0, len(campaigns))
	for _, campaign := range campaigns {
		var awarded int64
		if err := DB.Model(&PaymentCampaignClaim{}).
			Where("campaign_id = ? AND (status = ? OR status = '')", campaign.ID, CampaignClaimStatusAwarded).
			Count(&awarded).Error; err != nil {
			return nil, err
		}
		var reserved int64
		if err := DB.Model(&PaymentCampaignClaim{}).
			Where("campaign_id = ? AND status = ? AND reserved_until > ?", campaign.ID, CampaignClaimStatusReserved, now).
			Count(&reserved).Error; err != nil {
			return nil, err
		}
		remaining := int64(0)
		limited := campaign.MaxClaimsTotal > 0
		if limited {
			remaining = int64(campaign.MaxClaimsTotal) - awarded - reserved
			if remaining < 0 {
				remaining = 0
			}
		}
		stats = append(stats, PaymentCampaignClaimStats{
			CampaignId: campaign.ID,
			Limited:    limited,
			Total:      int64(campaign.MaxClaimsTotal),
			Awarded:    awarded,
			Reserved:   reserved,
			Remaining:  remaining,
		})
	}
	return stats, nil
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
