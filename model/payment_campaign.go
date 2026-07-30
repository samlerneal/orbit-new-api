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
	CampaignId                  string `json:"campaign_id" gorm:"type:varchar(64);primaryKey"`
	Revision                    int64  `json:"revision" gorm:"type:bigint;not null;default:0"`
	ParticipantMigrationVersion int    `json:"participant_migration_version" gorm:"type:int;not null;default:0"`
	UpdatedAt                   int64  `json:"updated_at" gorm:"type:bigint;not null;default:0"`
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
	CampaignId           string `json:"campaign_id"`
	CampaignName         string `json:"campaign_name"`
	Eligibility          string `json:"eligibility"`
	MaxClaims            int    `json:"max_claims"`
	MaxClaimsPerEmail    int    `json:"max_claims_per_email"`
	MaxClaimsTotal       int    `json:"max_claims_total"`
	MaxParticipantsTotal int    `json:"max_participants_total"`
	ReservationMinutes   int    `json:"reservation_minutes"`
	PackageId            string `json:"package_id"`
	BonusQuota           int64  `json:"bonus_quota"`
	BonusAmountCNY       string `json:"bonus_amount_cny"`
	ValidDays            int    `json:"valid_days"`
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
		query := campaignActiveClaimQuery(DB, campaign.ID, now).
			Where("email_hash = ?", emailHash)
		if campaign.Eligibility == operation_setting.CampaignEligibilityPerPackage {
			query = query.Where("package_id = ?", packageId)
		}
		if err := query.Count(&count).Error; err != nil {
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

func ResolveCampaignReward(campaign operation_setting.PaymentCampaign, packageOption operation_setting.TopupPackage) (decimal.Decimal, decimal.Decimal, error) {
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
		totalAmount, bonusAmount, err := ResolveCampaignReward(campaign, packageOption)
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
			CampaignId:           offer.Campaign.ID,
			CampaignName:         offer.Campaign.Name,
			Eligibility:          offer.Campaign.Eligibility,
			MaxClaims:            offer.Campaign.MaxClaimsPerUser,
			MaxClaimsPerEmail:    offer.Campaign.MaxClaimsPerEmail,
			MaxClaimsTotal:       offer.Campaign.MaxClaimsTotal,
			MaxParticipantsTotal: offer.Campaign.MaxParticipantsTotal,
			ReservationMinutes:   offer.Campaign.ReservationMinutes,
			PackageId:            packageOption.ID,
			BonusQuota:           offer.BonusQuota,
			BonusAmountCNY:       decimal.NewFromFloat(offer.BonusAmount).StringFixed(2),
			ValidDays:            offer.Campaign.ValidDays,
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
	if err := tx.Model(&PaymentCampaignClaim{}).
		Where("campaign_id = ? AND status = ? AND reserved_until <= ?", campaignId, CampaignClaimStatusReserved, now).
		Updates(map[string]interface{}{
			"status":            CampaignClaimStatusReleased,
			"claim_key":         nil,
			"email_claim_key":   nil,
			"campaign_slot_key": nil,
			"released_at":       now,
			"updated_at":        now,
		}).Error; err != nil {
		return err
	}
	if err := releaseExpiredParticipantsLockedTx(tx, campaignId, now); err != nil {
		return err
	}
	return nil
}

func releaseExpiredParticipantsLockedTx(tx *gorm.DB, campaignId string, now int64) error {
	var expiredParticipants []PaymentCampaignParticipant
	if err := tx.Where("campaign_id = ? AND status = ? AND reserved_until <= ?", campaignId, CampaignParticipantStatusReserved, now).
		Find(&expiredParticipants).Error; err != nil {
		return err
	}
	for _, participant := range expiredParticipants {
		if err := tx.Model(&PaymentCampaignClaim{}).
			Where("campaign_id = ? AND user_id = ? AND status = ?", campaignId, participant.UserId, CampaignClaimStatusReserved).
			Updates(map[string]interface{}{
				"status":            CampaignClaimStatusReleased,
				"claim_key":         nil,
				"email_claim_key":   nil,
				"campaign_slot_key": nil,
				"released_at":       now,
				"updated_at":        now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&participant).Updates(map[string]interface{}{
			"status":                CampaignParticipantStatusReleased,
			"participant_key":       nil,
			"email_participant_key": nil,
			"campaign_slot_key":     nil,
			"released_at":           now,
			"updated_at":            now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
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

func campaignParticipantPrefix(campaign operation_setting.PaymentCampaign) string {
	return fmt.Sprintf("%s:participant", campaign.ID)
}

func campaignParticipantEmailPrefix(campaign operation_setting.PaymentCampaign, userId int, emailHash string) string {
	if campaign.Eligibility == operation_setting.CampaignEligibilityPerCampaign {
		return fmt.Sprintf("%s:user:%d:email:%s:participant", campaign.ID, userId, emailHash)
	}
	return fmt.Sprintf("%s:email:%s:participant", campaign.ID, emailHash)
}

func findOrCreateParticipantLockedTx(
	tx *gorm.DB,
	campaign operation_setting.PaymentCampaign,
	userId int,
	emailHash string,
	now int64,
) (*PaymentCampaignParticipant, bool, error) {
	var existing PaymentCampaignParticipant
	err := tx.Where("campaign_id = ? AND user_id = ?", campaign.ID, userId).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if err == nil {
		if existing.Status == CampaignParticipantStatusAdmitted {
			return &existing, false, nil
		}
		if existing.Status == CampaignParticipantStatusReserved && existing.ReservedUntil > now {
			return &existing, true, nil
		}
	}

	isNew := errors.Is(err, gorm.ErrRecordNotFound) || existing.Status == CampaignParticipantStatusReleased || existing.ReservedUntil <= now
	if !isNew {
		return &existing, existing.Status == CampaignParticipantStatusReserved, nil
	}

	if campaign.MaxParticipantsTotal > 0 {
		count, countErr := countActiveParticipantsTx(tx, campaign.ID, now)
		if countErr != nil {
			return nil, false, countErr
		}
		if count >= int64(campaign.MaxParticipantsTotal) {
			return nil, false, nil
		}
	}

	participantKeyVal := fmt.Sprintf("%s:user:%d", campaignParticipantPrefix(campaign), userId)
	participantKey := &participantKeyVal

	var emailParticipantKey *string
	if emailHash != "" {
		// per_campaign email limits are enforced by claim reservations across
		// the activity; participant identity remains account-scoped so that
		// distinct accounts sharing an email can exercise that limit.
		emailKeyVal := campaignParticipantEmailPrefix(campaign, userId, emailHash)
		emailParticipantKey = &emailKeyVal
		// Reject when another user already holds this email participant key.
		var existingByEmail PaymentCampaignParticipant
		emailErr := tx.Where("campaign_id = ? AND email_participant_key = ? AND user_id != ?",
			campaign.ID, emailKeyVal, userId).First(&existingByEmail).Error
		if emailErr == nil {
			return nil, false, nil
		}
		if !errors.Is(emailErr, gorm.ErrRecordNotFound) {
			return nil, false, emailErr
		}
	}

	var campaignSlotKey *string
	if campaign.MaxParticipantsTotal > 0 {
		slotKey, slotErr := firstAvailableCampaignParticipantSlotKeyTx(tx, campaign.ID, campaign.MaxParticipantsTotal, now)
		if slotErr != nil {
			return nil, false, slotErr
		}
		if slotKey == nil {
			return nil, false, nil
		}
		campaignSlotKey = slotKey
	}

	reservationMinutes := campaign.ReservationMinutes
	if reservationMinutes <= 0 {
		reservationMinutes = operation_setting.DefaultCampaignReservationMinutes
	}
	reservedUntil := now + int64(reservationMinutes)*60

	if !errors.Is(err, gorm.ErrRecordNotFound) && existing.Id > 0 {
		values := map[string]interface{}{
			"status":                CampaignParticipantStatusReserved,
			"email_hash":            emailHash,
			"participant_key":       participantKeyVal,
			"email_participant_key": emailParticipantKey,
			"campaign_slot_key":     campaignSlotKey,
			"reserved_until":        reservedUntil,
			"released_at":           0,
			"updated_at":            now,
		}
		if updateErr := tx.Model(&existing).Updates(values).Error; updateErr != nil {
			return nil, false, updateErr
		}

		var reRead PaymentCampaignParticipant
		if reReadErr := tx.Where("campaign_id = ? AND user_id = ?", campaign.ID, userId).First(&reRead).Error; reReadErr != nil {
			return nil, false, reReadErr
		}
		if reRead.Status != CampaignParticipantStatusReserved && reRead.Status != CampaignParticipantStatusAdmitted {
			return nil, false, errors.New("participant state inconsistent after reuse")
		}
		if reRead.ParticipantKey == nil || *reRead.ParticipantKey != participantKeyVal {
			return nil, false, errors.New("participant key inconsistent after reuse")
		}
		if campaign.MaxParticipantsTotal > 0 && (reRead.CampaignSlotKey == nil || *reRead.CampaignSlotKey != *campaignSlotKey) {
			return nil, false, errors.New("participant slot key inconsistent after reuse")
		}
		return &reRead, true, nil
	}

	participant := &PaymentCampaignParticipant{
		CampaignId:          campaign.ID,
		UserId:              userId,
		EmailHash:           emailHash,
		Status:              CampaignParticipantStatusReserved,
		ParticipantKey:      participantKey,
		EmailParticipantKey: emailParticipantKey,
		CampaignSlotKey:     campaignSlotKey,
		ReservedUntil:       reservedUntil,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if createErr := tx.Create(participant).Error; createErr != nil {
		return nil, false, createErr
	}

	var reRead PaymentCampaignParticipant
	if reReadErr := tx.Where("campaign_id = ? AND user_id = ?", campaign.ID, userId).First(&reRead).Error; reReadErr != nil {
		return nil, false, reReadErr
	}
	if reRead.Status != CampaignParticipantStatusReserved && reRead.Status != CampaignParticipantStatusAdmitted {
		return nil, false, errors.New("participant state inconsistent after create")
	}
	if reRead.ParticipantKey == nil || *reRead.ParticipantKey != *participantKey {
		return nil, false, errors.New("participant key inconsistent after create")
	}
	if campaign.MaxParticipantsTotal > 0 && (reRead.CampaignSlotKey == nil || *reRead.CampaignSlotKey != *campaignSlotKey) {
		return nil, false, errors.New("participant slot key inconsistent after create")
	}
	return &reRead, true, nil
}

func countActiveParticipantsTx(tx *gorm.DB, campaignId string, now int64) (int64, error) {
	var count int64
	err := tx.Model(&PaymentCampaignParticipant{}).
		Where("campaign_id = ? AND (status = ? OR (status = ? AND reserved_until > ?))",
			campaignId, CampaignParticipantStatusAdmitted, CampaignParticipantStatusReserved, now).
		Count(&count).Error
	return count, err
}

func firstAvailableCampaignParticipantSlotKeyTx(
	tx *gorm.DB,
	campaignId string,
	limit int,
	now int64,
) (*string, error) {
	if limit <= 0 {
		return nil, nil
	}
	var used []string
	if err := tx.Model(&PaymentCampaignParticipant{}).
		Where("campaign_id = ? AND campaign_slot_key IS NOT NULL AND (status = ? OR (status = ? AND reserved_until > ?))",
			campaignId, CampaignParticipantStatusAdmitted, CampaignParticipantStatusReserved, now).
		Pluck("campaign_slot_key", &used).Error; err != nil {
		return nil, err
	}
	usedSet := make(map[string]struct{}, len(used))
	for _, key := range used {
		usedSet[key] = struct{}{}
	}
	for slot := 1; slot <= limit; slot++ {
		key := fmt.Sprintf("%s:campaign-slot:%d", campaignId, slot)
		if _, exists := usedSet[key]; !exists {
			return &key, nil
		}
	}
	return nil, nil
}

func buildCampaignClaimKeysForPackageTx(
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
			Where("email_hash = ? AND package_id = ?", emailHash, topUp.PackageId).
			Count(&count).Error
		if err != nil || count >= int64(campaign.MaxClaimsPerEmail) {
			return campaignReservationKeys{}, false, err
		}
	}
	emailPrefix := fmt.Sprintf("%s:email:%s", campaign.ID, emailHash)
	if campaign.Eligibility == operation_setting.CampaignEligibilityPerPackage {
		emailPrefix = fmt.Sprintf("%s:package:%s", emailPrefix, topUp.PackageId)
	}
	emailKey, err := firstAvailableCampaignKeyTx(
		tx, campaign.ID, "email_claim_key", campaign.MaxClaimsPerEmail, now,
		func(slot int) string {
			return fmt.Sprintf("%s:slot:%d", emailPrefix, slot)
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
	maxReservedUntil int64,
) (*PaymentCampaignClaim, error) {
	reservationMinutes := campaign.ReservationMinutes
	if reservationMinutes <= 0 {
		reservationMinutes = operation_setting.DefaultCampaignReservationMinutes
	}
	reservedUntil := now + int64(reservationMinutes)*60
	if maxReservedUntil > 0 && reservedUntil > maxReservedUntil {
		reservedUntil = maxReservedUntil
	}
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

func reserveCampaignClaimForPackageLockedTx(
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

	keys, available, err := buildCampaignClaimKeysForPackageTx(tx, topUp, campaign, emailHash, now)
	if err != nil || !available {
		return nil, false, err
	}

	var maxReservedUntil int64
	if campaign.MaxParticipantsTotal > 0 {
		var participant PaymentCampaignParticipant
		participantErr := tx.Where("campaign_id = ? AND user_id = ?", campaign.ID, topUp.UserId).First(&participant).Error
		if participantErr == nil &&
			participant.Status == CampaignParticipantStatusReserved &&
			participant.ReservedUntil > now {
			maxReservedUntil = participant.ReservedUntil
		}
	}

	claim, err := saveCampaignReservationTx(tx, &existing, topUp, campaign, emailHash, keys, now, maxReservedUntil)
	return claim, err == nil, err
}

func syncSnapshotSafeguards(snapshot CampaignAwardSnapshot, campaign operation_setting.PaymentCampaign) CampaignAwardSnapshot {
	snapshot.Eligibility = campaign.Eligibility
	snapshot.MaxClaims = campaign.MaxClaimsPerUser
	snapshot.MaxClaimsPerEmail = campaign.MaxClaimsPerEmail
	snapshot.MaxClaimsTotal = campaign.MaxClaimsTotal
	snapshot.MaxParticipantsTotal = campaign.MaxParticipantsTotal
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

			participant, isNewParticipant, err := findOrCreateParticipantLockedTx(tx, campaign, topUp.UserId, emailHash, now)
			if err != nil {
				return err
			}
			if participant == nil && campaign.MaxParticipantsTotal > 0 {
				return ErrCampaignReservationUnavailable
			}

			_, didReserve, err := reserveCampaignClaimForPackageLockedTx(tx, topUp, campaign, emailHash, now)
			if err != nil {
				return err
			}
			if didReserve {
				if isNewParticipant {
					_ = participant
				}
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
		if snapshot.MaxClaimsTotal == 0 && snapshot.MaxParticipantsTotal == 0 && snapshot.ReservationMinutes == 0 && errors.Is(err, gorm.ErrRecordNotFound) {
			campaign = legacyCampaignFromSnapshot(snapshot)
		} else {
			return nil, false, nil
		}
	}

	var participant *PaymentCampaignParticipant
	if campaign.MaxParticipantsTotal > 0 {
		tx.Where("campaign_id = ? AND user_id = ?", campaign.ID, topUp.UserId).First(&participant)
		if participant == nil || participant.Id == 0 {
			participant, _, err = findOrCreateParticipantLockedTx(tx, campaign, topUp.UserId, emailHash, now)
			if err != nil || participant == nil {
				return nil, false, err
			}
			return reserveCampaignClaimForPackageLockedTx(tx, topUp, campaign, emailHash, now)
		}
		if participant.Status == CampaignParticipantStatusAdmitted {
			return reserveCampaignClaimForPackageLockedTx(tx, topUp, campaign, emailHash, now)
		}
		if participant.Status == CampaignParticipantStatusReserved && participant.ReservedUntil > now {
			return reserveCampaignClaimForPackageLockedTx(tx, topUp, campaign, emailHash, now)
		}
		return nil, false, nil
	}

	return reserveCampaignClaimForPackageLockedTx(tx, topUp, campaign, emailHash, now)
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

		if snapshot.MaxParticipantsTotal > 0 {
			participantResult := tx.Model(&PaymentCampaignParticipant{}).
				Where("campaign_id = ? AND user_id = ? AND status = ?", snapshot.CampaignId, topUp.UserId, CampaignParticipantStatusReserved).
				Updates(map[string]interface{}{
					"status":                CampaignParticipantStatusAdmitted,
					"admitted_at":           now,
					"reserved_until":        0,
					"email_participant_key": gorm.Expr("email_participant_key"),
					"campaign_slot_key":     gorm.Expr("campaign_slot_key"),
					"updated_at":            now,
				})
			if participantResult.Error != nil {
				return awarded, participantResult.Error
			}
		}
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

		var remaining int64
		if err := tx.Model(&PaymentCampaignClaim{}).
			Where("campaign_id = ? AND user_id = ? AND status = ? AND topup_id != ? AND reserved_until > ?",
				claim.CampaignId, claim.UserId, CampaignClaimStatusReserved, topUpId, now).
			Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			if err := tx.Model(&PaymentCampaignParticipant{}).
				Where("campaign_id = ? AND user_id = ? AND status = ?",
					claim.CampaignId, claim.UserId, CampaignParticipantStatusReserved).
				Updates(map[string]interface{}{
					"status":                CampaignParticipantStatusReleased,
					"participant_key":       nil,
					"email_participant_key": nil,
					"campaign_slot_key":     nil,
					"released_at":           now,
					"updated_at":            now,
				}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

type PaymentCampaignClaimStats struct {
	CampaignId            string `json:"campaign_id"`
	ParticipantLimited    bool   `json:"participant_limited"`
	ParticipantsTotal     int64  `json:"participants_total"`
	ParticipantsAdmitted  int64  `json:"participants_admitted"`
	ParticipantsReserved  int64  `json:"participants_reserved"`
	ParticipantsRemaining int64  `json:"participants_remaining"`
	Limited               bool   `json:"limited"`
	Total                 int64  `json:"total"`
	Awarded               int64  `json:"awarded"`
	Reserved              int64  `json:"reserved"`
	Remaining             int64  `json:"remaining"`
	ClaimsAwarded         int64  `json:"claims_awarded"`
	ClaimsReserved        int64  `json:"claims_reserved"`
}

func GetPaymentCampaignClaimStats(now int64) ([]PaymentCampaignClaimStats, error) {
	campaigns := operation_setting.GetPaymentCampaigns()
	stats := make([]PaymentCampaignClaimStats, 0, len(campaigns))
	for _, campaign := range campaigns {
		var admitted int64
		if err := DB.Model(&PaymentCampaignParticipant{}).
			Where("campaign_id = ? AND status = ?", campaign.ID, CampaignParticipantStatusAdmitted).
			Count(&admitted).Error; err != nil {
			return nil, err
		}
		var reservedParticipants int64
		if err := DB.Model(&PaymentCampaignParticipant{}).
			Where("campaign_id = ? AND status = ? AND reserved_until > ?", campaign.ID, CampaignParticipantStatusReserved, now).
			Count(&reservedParticipants).Error; err != nil {
			return nil, err
		}

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

		participantLimited := campaign.MaxParticipantsTotal > 0
		participantsTotal := int64(campaign.MaxParticipantsTotal)
		participantsRemaining := int64(0)
		if participantLimited {
			participantsRemaining = participantsTotal - admitted - reservedParticipants
			if participantsRemaining < 0 {
				participantsRemaining = 0
			}
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
			CampaignId:            campaign.ID,
			ParticipantLimited:    participantLimited,
			ParticipantsTotal:     participantsTotal,
			ParticipantsAdmitted:  admitted,
			ParticipantsReserved:  reservedParticipants,
			ParticipantsRemaining: participantsRemaining,
			Limited:               limited,
			Total:                 int64(campaign.MaxClaimsTotal),
			Awarded:               awarded,
			Reserved:              reserved,
			Remaining:             remaining,
			ClaimsAwarded:         awarded,
			ClaimsReserved:        reserved,
		})
	}
	return stats, nil
}

type BonusBalanceSummary struct {
	ActiveQuota      int64 `json:"active_quota"`
	NearestExpiresAt int64 `json:"nearest_expires_at"`
}

type bonusBalanceSummaryRow struct {
	UserId           int   `gorm:"column:user_id"`
	ActiveQuota      int64 `gorm:"column:active_quota"`
	NearestExpiresAt int64 `gorm:"column:nearest_expires_at"`
}

// migratePaymentCampaignParticipants backfills PaymentCampaignParticipant
// rows from existing awarded/reserved PaymentCampaignClaim records per
// campaign. The migration is idempotent: ParticipantMigrationVersion is
// checked before backfill and updated only after full success within a
// single transaction per campaign. Failure prevents service startup.
func migratePaymentCampaignParticipants() error {
	campaigns := operation_setting.GetPaymentCampaigns()
	now := common.GetTimestamp()
	for _, campaign := range campaigns {
		err := DB.Transaction(func(tx *gorm.DB) error {
			if err := lockCampaignStateTx(tx, campaign.ID, now); err != nil {
				return err
			}
			var state PaymentCampaignState
			if err := tx.Where("campaign_id = ?", campaign.ID).First(&state).Error; err != nil {
				return err
			}
			if state.ParticipantMigrationVersion >= 1 {
				return nil
			}
			if err := migrateActivityParticipantsTx(tx, campaign, now); err != nil {
				return err
			}
			return tx.Model(&state).Update("participant_migration_version", 1).Error
		})
		if err != nil {
			return fmt.Errorf("campaign participant migration failed for %s: %w", campaign.ID, err)
		}
	}
	return nil
}

type participantMigrationSource struct {
	UserId        int
	EmailHash     string
	ReservedUntil int64
	AdmittedAt    int64
}

func loadParticipantMigrationSourcesTx(
	tx *gorm.DB,
	campaignId string,
	status string,
	now int64,
) ([]participantMigrationSource, error) {
	var claims []PaymentCampaignClaim
	query := tx.Model(&PaymentCampaignClaim{}).
		Select("id", "user_id", "email_hash", "reserved_until", "awarded_at", "created_at").
		Where("campaign_id = ?", campaignId).
		Order("user_id ASC, created_at ASC, id ASC")
	if status == CampaignClaimStatusAwarded {
		query = query.Where("status = ? OR status = ''", CampaignClaimStatusAwarded)
	} else {
		query = query.Where("status = ? AND reserved_until > ?", CampaignClaimStatusReserved, now)
	}
	if err := query.Find(&claims).Error; err != nil {
		return nil, err
	}

	sources := make([]participantMigrationSource, 0)
	sourceByUser := make(map[int]int)
	for _, claim := range claims {
		index, exists := sourceByUser[claim.UserId]
		if !exists {
			index = len(sources)
			sourceByUser[claim.UserId] = index
			sources = append(sources, participantMigrationSource{UserId: claim.UserId})
		}
		source := &sources[index]
		if claim.EmailHash != "" {
			if source.EmailHash != "" && source.EmailHash != claim.EmailHash {
				return nil, fmt.Errorf("campaign %s user %d has conflicting historical email hashes", campaignId, claim.UserId)
			}
			source.EmailHash = claim.EmailHash
		}
		if status == CampaignClaimStatusAwarded {
			admittedAt := claim.AwardedAt
			if admittedAt == 0 {
				admittedAt = claim.CreatedAt
			}
			if source.AdmittedAt == 0 || (admittedAt > 0 && admittedAt < source.AdmittedAt) {
				source.AdmittedAt = admittedAt
			}
		} else if claim.ReservedUntil > source.ReservedUntil {
			source.ReservedUntil = claim.ReservedUntil
		}
	}
	return sources, nil
}

func fillParticipantMigrationEmailTx(tx *gorm.DB, source *participantMigrationSource) error {
	if source.EmailHash != "" {
		return nil
	}
	var user User
	if err := tx.Select("email").First(&user, source.UserId).Error; err != nil {
		return err
	}
	source.EmailHash = CampaignEmailHash(user.Email)
	return nil
}

func nextMigrationParticipantSlotKeyTx(tx *gorm.DB, campaignId string) (*string, error) {
	var used []string
	if err := tx.Model(&PaymentCampaignParticipant{}).
		Where("campaign_id = ? AND campaign_slot_key IS NOT NULL", campaignId).
		Pluck("campaign_slot_key", &used).Error; err != nil {
		return nil, err
	}
	usedSet := make(map[string]struct{}, len(used))
	for _, key := range used {
		usedSet[key] = struct{}{}
	}
	for slot := 1; ; slot++ {
		key := fmt.Sprintf("%s:campaign-slot:%d", campaignId, slot)
		if _, exists := usedSet[key]; !exists {
			return &key, nil
		}
	}
}

func upsertParticipantMigrationSourceTx(
	tx *gorm.DB,
	campaign operation_setting.PaymentCampaign,
	source participantMigrationSource,
	status string,
	now int64,
) error {
	var existing PaymentCampaignParticipant
	err := tx.Where("campaign_id = ? AND user_id = ?", campaign.ID, source.UserId).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	notFound := errors.Is(err, gorm.ErrRecordNotFound)
	if source.EmailHash == "" && !notFound {
		source.EmailHash = existing.EmailHash
	}
	if err := fillParticipantMigrationEmailTx(tx, &source); err != nil {
		return err
	}
	if !notFound && existing.EmailHash != "" && existing.EmailHash != source.EmailHash {
		return fmt.Errorf(
			"campaign %s user %d participant email does not match historical claim",
			campaign.ID,
			source.UserId,
		)
	}
	if !notFound && existing.Status == CampaignParticipantStatusAdmitted &&
		status == CampaignParticipantStatusReserved {
		status = CampaignParticipantStatusAdmitted
		source.ReservedUntil = 0
		if source.AdmittedAt == 0 {
			source.AdmittedAt = existing.AdmittedAt
		}
	}

	participantKey := ptrStr(fmt.Sprintf("%s:user:%d", campaignParticipantPrefix(campaign), source.UserId))
	var emailParticipantKey *string
	if source.EmailHash != "" {
		emailParticipantKey = ptrStr(campaignParticipantEmailPrefix(campaign, source.UserId, source.EmailHash))
	}
	campaignSlotKey := existing.CampaignSlotKey
	if campaign.MaxParticipantsTotal > 0 && campaignSlotKey == nil {
		campaignSlotKey, err = nextMigrationParticipantSlotKeyTx(tx, campaign.ID)
		if err != nil {
			return err
		}
	}

	admittedAt := source.AdmittedAt
	if status == CampaignParticipantStatusAdmitted && admittedAt == 0 {
		admittedAt = now
	}
	values := map[string]interface{}{
		"email_hash":            source.EmailHash,
		"status":                status,
		"participant_key":       participantKey,
		"email_participant_key": emailParticipantKey,
		"campaign_slot_key":     campaignSlotKey,
		"reserved_until":        source.ReservedUntil,
		"admitted_at":           admittedAt,
		"released_at":           0,
		"updated_at":            now,
	}
	if notFound {
		participant := &PaymentCampaignParticipant{
			CampaignId:          campaign.ID,
			UserId:              source.UserId,
			EmailHash:           source.EmailHash,
			Status:              status,
			ParticipantKey:      participantKey,
			EmailParticipantKey: emailParticipantKey,
			CampaignSlotKey:     campaignSlotKey,
			ReservedUntil:       source.ReservedUntil,
			AdmittedAt:          admittedAt,
			ReleasedAt:          0,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		return tx.Create(participant).Error
	}
	return tx.Model(&existing).Updates(values).Error
}

func migrateActivityParticipantsTx(tx *gorm.DB, campaign operation_setting.PaymentCampaign, now int64) error {
	admitted, err := loadParticipantMigrationSourcesTx(tx, campaign.ID, CampaignClaimStatusAwarded, now)
	if err != nil {
		return err
	}
	reserved, err := loadParticipantMigrationSourcesTx(tx, campaign.ID, CampaignClaimStatusReserved, now)
	if err != nil {
		return err
	}

	admittedUsers := make(map[int]struct{}, len(admitted))
	for _, source := range admitted {
		admittedUsers[source.UserId] = struct{}{}
		if err := upsertParticipantMigrationSourceTx(tx, campaign, source, CampaignParticipantStatusAdmitted, now); err != nil {
			return err
		}
	}
	for _, source := range reserved {
		if _, alreadyAdmitted := admittedUsers[source.UserId]; alreadyAdmitted {
			continue
		}
		if err := upsertParticipantMigrationSourceTx(tx, campaign, source, CampaignParticipantStatusReserved, now); err != nil {
			return err
		}
	}
	return nil
}

func ptrStr(s string) *string { return &s }

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

// GetBonusBalanceSummaries returns active bonus balances for a page of users
// with one grouped read. The caller supplies now so all rows in one response
// use the same expiry boundary.
func GetBonusBalanceSummaries(userIds []int, now int64) (map[int]BonusBalanceSummary, error) {
	summaries := make(map[int]BonusBalanceSummary, len(userIds))
	if len(userIds) == 0 {
		return summaries, nil
	}

	rows := make([]bonusBalanceSummaryRow, 0, len(userIds))
	err := DB.Model(&BonusBalance{}).
		Select("user_id, SUM(amount_total - amount_used) AS active_quota, MIN(expires_at) AS nearest_expires_at").
		Where("user_id IN ? AND status = ? AND expires_at > ? AND amount_total > amount_used", userIds, BonusBalanceStatusActive, now).
		Group("user_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		summaries[row.UserId] = BonusBalanceSummary{
			ActiveQuota:      row.ActiveQuota,
			NearestExpiresAt: row.NearestExpiresAt,
		}
	}
	return summaries, nil
}
