package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useTestCampaign(t *testing.T, campaign operation_setting.PaymentCampaign) {
	t.Helper()
	setting := operation_setting.GetPaymentSetting()
	previous := setting.Campaigns
	setting.Campaigns = []operation_setting.PaymentCampaign{campaign}
	t.Cleanup(func() {
		setting.Campaigns = previous
	})
}

func insertPaymentCampaignUser(t *testing.T, id int, user User) {
	t.Helper()
	require.NoError(t, DB.Exec(`INSERT INTO users (id, username, password, email, aff_code, status) VALUES (?, ?, ?, ?, ?, ?)`,
		id, user.Username, "password", user.Email, user.AffCode, common.UserStatusEnabled).Error)
	user.Id = id
}

func campaignTestRule(id string, total int) operation_setting.PaymentCampaign {
	return operation_setting.PaymentCampaign{
		ID:                   id,
		Name:                 "Campaign",
		Enabled:              true,
		PackageIDs:           []string{"advanced"},
		Eligibility:          operation_setting.CampaignEligibilityPerPackage,
		MaxClaimsPerUser:     1,
		MaxClaimsPerEmail:    1,
		MaxClaimsTotal:       0,
		MaxParticipantsTotal: total,
		ReservationMinutes:   3,
		RewardMode:           operation_setting.CampaignRewardTargetTotalPercent,
		RewardPercent:        30,
		RoundingMode:         operation_setting.CampaignRoundingCeilYuan,
		ValidDays:            45,
	}
}

func campaignTestSnapshot(campaign operation_setting.PaymentCampaign) CampaignAwardSnapshot {
	return CampaignAwardSnapshot{
		CampaignId:           campaign.ID,
		CampaignName:         campaign.Name,
		Eligibility:          campaign.Eligibility,
		MaxClaims:            campaign.MaxClaimsPerUser,
		MaxClaimsPerEmail:    campaign.MaxClaimsPerEmail,
		MaxClaimsTotal:       campaign.MaxClaimsTotal,
		MaxParticipantsTotal: campaign.MaxParticipantsTotal,
		ReservationMinutes:   campaign.ReservationMinutes,
		PackageId:            "advanced",
		BonusQuota:           280,
		BonusAmountCNY:       "28.00",
		ValidDays:            45,
	}
}

func createCampaignTestTopUp(
	t *testing.T,
	user User,
	campaign operation_setting.PaymentCampaign,
	tradeNo string,
	now int64,
) TopUp {
	t.Helper()
	if user.AffCode == "" {
		user.AffCode = user.Username
	}
	require.NoError(t, DB.Create(&user).Error)
	topUp := TopUp{
		UserId:          user.Id,
		PackageId:       "advanced",
		CreditQuota:     1_000,
		Money:           98,
		TradeNo:         tradeNo,
		PaymentMethod:   "wxpay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      now,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, CreateTopUpWithCampaignReservations(
		&topUp,
		[]CampaignAwardSnapshot{campaignTestSnapshot(campaign)},
		now,
	))
	return topUp
}

func createCampaignTestTopUpForPackage(
	t *testing.T,
	user User,
	campaign operation_setting.PaymentCampaign,
	packageID string,
	tradeNo string,
	now int64,
) TopUp {
	t.Helper()
	if user.AffCode == "" {
		user.AffCode = user.Username
	}
	require.NoError(t, DB.Create(&user).Error)
	topUp := TopUp{
		UserId: user.Id, PackageId: packageID, CreditQuota: 1_000,
		Money: 98, TradeNo: tradeNo, PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, CreateTime: now,
		Status: common.TopUpStatusPending,
	}
	snapshot := campaignTestSnapshot(campaign)
	snapshot.PackageId = packageID
	require.NoError(t, CreateTopUpWithCampaignReservations(&topUp, []CampaignAwardSnapshot{snapshot}, now))
	return topUp
}

func TestCampaignRewardUsesPaymentAmountAndRoundsUp(t *testing.T) {
	campaign := operation_setting.PaymentCampaign{
		RewardMode:    operation_setting.CampaignRewardTargetTotalPercent,
		RewardPercent: 30,
		RoundingMode:  operation_setting.CampaignRoundingCeilYuan,
	}
	tests := []struct {
		pay     float64
		regular float64
		total   string
		bonus   string
	}{
		{pay: 14, regular: 14, total: "19", bonus: "5"},
		{pay: 49, regular: 50, total: "64", bonus: "14"},
		{pay: 98, regular: 100, total: "128", bonus: "28"},
		{pay: 490, regular: 500, total: "637", bonus: "137"},
	}

	for _, test := range tests {
		t.Run(decimal.NewFromFloat(test.pay).String(), func(t *testing.T) {
			total, bonus, err := ResolveCampaignReward(campaign, operation_setting.TopupPackage{
				PayAmount: test.pay, CreditAmount: test.regular,
			})
			require.NoError(t, err)
			assert.True(t, total.Equal(decimal.RequireFromString(test.total)))
			assert.True(t, bonus.Equal(decimal.RequireFromString(test.bonus)))
		})
	}
}

func TestCampaignEmailHashNormalizesCaseAndWhitespace(t *testing.T) {
	assert.Equal(
		t,
		CampaignEmailHash(" User@Example.COM "),
		CampaignEmailHash("user@example.com"),
	)
	assert.Empty(t, CampaignEmailHash("  "))
}

type legacyPaymentCampaignClaim struct {
	Id         int    `gorm:"primaryKey"`
	CampaignId string `gorm:"type:varchar(64);uniqueIndex:idx_campaign_topup,priority:1"`
	UserId     int
	PackageId  string
	TopUpId    int     `gorm:"column:topup_id;uniqueIndex:idx_campaign_topup,priority:2"`
	ClaimKey   *string `gorm:"type:varchar(255);uniqueIndex"`
	CreatedAt  int64
}

func (legacyPaymentCampaignClaim) TableName() string {
	return "payment_campaign_claims"
}

func TestPaymentCampaignClaimMigrationPreservesLegacyRows(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(&legacyPaymentCampaignClaim{}))
	key := "legacy:user:1:slot:1"
	legacy := legacyPaymentCampaignClaim{
		CampaignId: "legacy",
		UserId:     1,
		PackageId:  "advanced",
		TopUpId:    1,
		ClaimKey:   &key,
		CreatedAt:  1,
	}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.AutoMigrate(&PaymentCampaignState{}, &PaymentCampaignClaim{}))
	require.NoError(t, ensurePaymentCampaignClaimIndexes(db))
	assert.True(t, db.Migrator().HasIndex(&PaymentCampaignClaim{}, "idx_campaign_email_claim_key"))
	assert.True(t, db.Migrator().HasIndex(&PaymentCampaignClaim{}, "idx_campaign_slot_key"))

	var migrated PaymentCampaignClaim
	require.NoError(t, db.First(&migrated, legacy.Id).Error)
	assert.Equal(t, CampaignClaimStatusAwarded, migrated.Status)
	assert.Empty(t, migrated.EmailHash)
	assert.Zero(t, migrated.ReservedUntil)
	assert.Equal(t, key, *migrated.ClaimKey)
}

func TestMigratePaymentCampaignParticipantsPreservesIdentityDeadlineAndIsIdempotent(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("participant-migration", 100)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()

	admittedUser := User{
		Username: "migration-admitted",
		Email:    "admitted@example.com",
		AffCode:  "migration-admitted",
	}
	reservedUser := User{
		Username: "migration-reserved",
		Email:    "reserved@example.com",
		AffCode:  "migration-reserved",
	}
	insertPaymentCampaignUser(t, 1, admittedUser)
	admittedUser.Id = 1
	insertPaymentCampaignUser(t, 2, reservedUser)
	reservedUser.Id = 2
	admittedEmailHash := CampaignEmailHash(admittedUser.Email)
	reservedEmailHash := CampaignEmailHash(reservedUser.Email)
	claims := []PaymentCampaignClaim{
		{
			CampaignId: campaign.ID, UserId: admittedUser.Id, PackageId: "standard",
			TopUpId: 1001, Status: CampaignClaimStatusAwarded, EmailHash: admittedEmailHash,
			AwardedAt: now - 120, CreatedAt: now - 130,
		},
		{
			CampaignId: campaign.ID, UserId: admittedUser.Id, PackageId: "advanced",
			TopUpId: 1002, Status: CampaignClaimStatusAwarded, EmailHash: admittedEmailHash,
			AwardedAt: now - 60, CreatedAt: now - 70,
		},
		{
			CampaignId: campaign.ID, UserId: reservedUser.Id, PackageId: "standard",
			TopUpId: 2001, Status: CampaignClaimStatusReserved, EmailHash: reservedEmailHash,
			ReservedUntil: now + 60, CreatedAt: now - 10,
		},
		{
			CampaignId: campaign.ID, UserId: reservedUser.Id, PackageId: "advanced",
			TopUpId: 2002, Status: CampaignClaimStatusReserved, EmailHash: reservedEmailHash,
			ReservedUntil: now + 120, CreatedAt: now - 5,
		},
	}
	require.NoError(t, DB.Create(&claims).Error)

	require.NoError(t, migratePaymentCampaignParticipants())

	var admitted PaymentCampaignParticipant
	require.NoError(t, DB.Where(
		"campaign_id = ? AND user_id = ?",
		campaign.ID,
		admittedUser.Id,
	).First(&admitted).Error)
	assert.Equal(t, CampaignParticipantStatusAdmitted, admitted.Status)
	assert.Equal(t, admittedEmailHash, admitted.EmailHash)
	require.NotNil(t, admitted.ParticipantKey)
	assert.Equal(t, campaignParticipantPrefix(campaign)+fmt.Sprintf(":user:%d", admittedUser.Id), *admitted.ParticipantKey)
	require.NotNil(t, admitted.EmailParticipantKey)
	assert.Equal(t, campaignParticipantEmailPrefix(campaign, admittedUser.Id, admittedEmailHash), *admitted.EmailParticipantKey)
	require.NotNil(t, admitted.CampaignSlotKey)
	assert.Equal(t, now-120, admitted.AdmittedAt)
	assert.Zero(t, admitted.ReservedUntil)

	var reserved PaymentCampaignParticipant
	require.NoError(t, DB.Where(
		"campaign_id = ? AND user_id = ?",
		campaign.ID,
		reservedUser.Id,
	).First(&reserved).Error)
	assert.Equal(t, CampaignParticipantStatusReserved, reserved.Status)
	assert.Equal(t, reservedEmailHash, reserved.EmailHash)
	require.NotNil(t, reserved.ParticipantKey)
	require.NotNil(t, reserved.EmailParticipantKey)
	assert.Equal(t, campaignParticipantEmailPrefix(campaign, reservedUser.Id, reservedEmailHash), *reserved.EmailParticipantKey)
	require.NotNil(t, reserved.CampaignSlotKey)
	assert.Equal(t, now+120, reserved.ReservedUntil)
	assert.Zero(t, reserved.AdmittedAt)

	var state PaymentCampaignState
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.ID).Error)
	assert.Equal(t, 1, state.ParticipantMigrationVersion)
	firstRevision := state.Revision
	firstAdmittedSlot := *admitted.CampaignSlotKey
	firstReservedSlot := *reserved.CampaignSlotKey

	require.NoError(t, migratePaymentCampaignParticipants())

	var participantCount int64
	require.NoError(t, DB.Model(&PaymentCampaignParticipant{}).
		Where("campaign_id = ?", campaign.ID).
		Count(&participantCount).Error)
	assert.EqualValues(t, 2, participantCount)
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.ID).Error)
	assert.Equal(t, firstRevision+1, state.Revision)
	require.NoError(t, DB.First(&admitted, admitted.Id).Error)
	require.NoError(t, DB.First(&reserved, reserved.Id).Error)
	assert.Equal(t, firstAdmittedSlot, *admitted.CampaignSlotKey)
	assert.Equal(t, firstReservedSlot, *reserved.CampaignSlotKey)
	assert.Equal(t, now+120, reserved.ReservedUntil)
}

func TestMigratePerCampaignParticipantsAllowsSameEmailAcrossHistoricalPackages(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("per-campaign-participant-migration", 30)
	campaign.Eligibility = operation_setting.CampaignEligibilityPerCampaign
	campaign.MaxClaimsPerUser = 1
	campaign.MaxClaimsPerEmail = 2
	campaign.PackageIDs = []string{"advanced", "professional"}
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	firstUser := User{Username: "migration-shared-email-a", Email: "migration-shared@example.com", AffCode: "migration-shared-email-a"}
	secondUser := User{Username: "migration-shared-email-b", Email: "migration-shared@example.com", AffCode: "migration-shared-email-b"}
	insertPaymentCampaignUser(t, 1, firstUser)
	firstUser.Id = 1
	insertPaymentCampaignUser(t, 2, secondUser)
	secondUser.Id = 2
	sharedHash := CampaignEmailHash(firstUser.Email)
	claims := []PaymentCampaignClaim{
		{CampaignId: campaign.ID, UserId: firstUser.Id, PackageId: "advanced", TopUpId: 3101, Status: CampaignClaimStatusAwarded, EmailHash: sharedHash, AwardedAt: now - 20, CreatedAt: now - 30},
		{CampaignId: campaign.ID, UserId: secondUser.Id, PackageId: "professional", TopUpId: 3102, Status: CampaignClaimStatusAwarded, EmailHash: sharedHash, AwardedAt: now - 10, CreatedAt: now - 15},
	}
	require.NoError(t, DB.Create(&claims).Error)

	require.NoError(t, migratePaymentCampaignParticipants())
	var participants []PaymentCampaignParticipant
	require.NoError(t, DB.Where("campaign_id = ?", campaign.ID).Order("user_id ASC").Find(&participants).Error)
	require.Len(t, participants, 2)
	assert.Equal(t, campaignParticipantEmailPrefix(campaign, firstUser.Id, sharedHash), *participants[0].EmailParticipantKey)
	assert.Equal(t, campaignParticipantEmailPrefix(campaign, secondUser.Id, sharedHash), *participants[1].EmailParticipantKey)
	var state PaymentCampaignState
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.ID).Error)
	assert.Equal(t, 1, state.ParticipantMigrationVersion)
}

func TestMigratePaymentCampaignParticipantsRollsBackOnFailure(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("participant-migration-rollback", 100)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()

	firstUser := User{
		Username: "migration-first",
		Email:    "migration-first@example.com",
		AffCode:  "migration-first",
	}
	secondUser := User{
		Username: "migration-second",
		Email:    "migration-second@example.com",
		AffCode:  "migration-second",
	}
	insertPaymentCampaignUser(t, 1, firstUser)
	firstUser.Id = 1
	insertPaymentCampaignUser(t, 2, secondUser)
	secondUser.Id = 2
	require.NoError(t, DB.Create(&[]PaymentCampaignClaim{
		{
			CampaignId: campaign.ID, UserId: firstUser.Id, PackageId: "experience",
			TopUpId: 3001, Status: CampaignClaimStatusAwarded,
			EmailHash: CampaignEmailHash(firstUser.Email), AwardedAt: now - 20,
		},
		{
			CampaignId: campaign.ID, UserId: secondUser.Id, PackageId: "standard",
			TopUpId: 3002, Status: CampaignClaimStatusAwarded,
			EmailHash: CampaignEmailHash(secondUser.Email), AwardedAt: now - 10,
		},
	}).Error)
	triggerSQL := fmt.Sprintf(`
		CREATE TRIGGER reject_participant_migration
		BEFORE INSERT ON payment_campaign_participants
		WHEN NEW.user_id = %d
		BEGIN
			SELECT RAISE(ABORT, 'injected participant migration failure');
		END
	`, secondUser.Id)
	require.NoError(t, DB.Exec(triggerSQL).Error)

	err := migratePaymentCampaignParticipants()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected participant migration failure")

	var participantCount int64
	require.NoError(t, DB.Model(&PaymentCampaignParticipant{}).
		Where("campaign_id = ?", campaign.ID).
		Count(&participantCount).Error)
	assert.Zero(t, participantCount)
	var migratedStateCount int64
	require.NoError(t, DB.Model(&PaymentCampaignState{}).
		Where("campaign_id = ? AND participant_migration_version >= ?", campaign.ID, 1).
		Count(&migratedStateCount).Error)
	assert.Zero(t, migratedStateCount)

	require.NoError(t, DB.Exec("DROP TRIGGER reject_participant_migration").Error)
	require.NoError(t, migratePaymentCampaignParticipants())

	var migratedParticipants []PaymentCampaignParticipant
	require.NoError(t, DB.Where("campaign_id = ?", campaign.ID).
		Order("user_id ASC").
		Find(&migratedParticipants).Error)
	require.Len(t, migratedParticipants, 2)
	require.NotNil(t, migratedParticipants[0].CampaignSlotKey)
	require.NotNil(t, migratedParticipants[1].CampaignSlotKey)
	assert.Equal(t, campaign.ID+":campaign-slot:1", *migratedParticipants[0].CampaignSlotKey)
	assert.Equal(t, campaign.ID+":campaign-slot:2", *migratedParticipants[1].CampaignSlotKey)
	var migratedState PaymentCampaignState
	require.NoError(t, DB.First(&migratedState, "campaign_id = ?", campaign.ID).Error)
	assert.Equal(t, 1, migratedState.ParticipantMigrationVersion)
}

func TestCampaignTotalLimitReservesFirstHundredAndRejectsNext(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("limit-100", 100)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()

	for index := 1; index <= 100; index++ {
		createCampaignTestTopUp(
			t,
			User{Username: fmt.Sprintf("limit-user-%d", index), Email: fmt.Sprintf("limit-%d@example.com", index)},
			campaign,
			fmt.Sprintf("limit-trade-%d", index),
			now,
		)
	}

	user := User{Username: "limit-user-101", Email: "limit-101@example.com", AffCode: "limit-user-101"}
	require.NoError(t, DB.Create(&user).Error)
	topUp := TopUp{
		UserId: user.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "limit-trade-101", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	err := CreateTopUpWithCampaignReservations(
		&topUp,
		[]CampaignAwardSnapshot{campaignTestSnapshot(campaign)},
		now,
	)
	assert.ErrorIs(t, err, ErrCampaignReservationUnavailable)

	var claimCount int64
	require.NoError(t, DB.Model(&PaymentCampaignClaim{}).
		Where("campaign_id = ? AND status = ?", campaign.ID, CampaignClaimStatusReserved).
		Count(&claimCount).Error)
	assert.EqualValues(t, 100, claimCount)
	var rolledBackOrderCount int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", topUp.TradeNo).Count(&rolledBackOrderCount).Error)
	assert.Zero(t, rolledBackOrderCount)
}

func TestCampaignConcurrentRequestsDoNotOverbookFinalSlot(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("concurrent-last-slot", 1)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	const contenderCount = 8
	users := make([]User, contenderCount)
	for index := range users {
		users[index] = User{
			Username: fmt.Sprintf("contender-%d", index),
			Email:    fmt.Sprintf("contender-%d@example.com", index),
			AffCode:  fmt.Sprintf("contender-%d", index),
		}
		require.NoError(t, DB.Create(&users[index]).Error)
	}

	start := make(chan struct{})
	results := make(chan error, contenderCount)
	var waitGroup sync.WaitGroup
	for index, user := range users {
		waitGroup.Add(1)
		go func(index int, user User) {
			defer waitGroup.Done()
			<-start
			topUp := TopUp{
				UserId: user.Id, PackageId: "advanced", CreditQuota: 1_000,
				Money: 98, TradeNo: fmt.Sprintf("contender-trade-%d", index),
				PaymentMethod: "wxpay", PaymentProvider: PaymentProviderEpay,
				Status: common.TopUpStatusPending,
			}
			results <- CreateTopUpWithCampaignReservations(
				&topUp,
				[]CampaignAwardSnapshot{campaignTestSnapshot(campaign)},
				now,
			)
		}(index, user)
	}
	close(start)
	waitGroup.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		assert.ErrorIs(t, err, ErrCampaignReservationUnavailable)
	}
	var claims int64
	require.NoError(t, DB.Model(&PaymentCampaignClaim{}).
		Where("campaign_id = ? AND status = ?", campaign.ID, CampaignClaimStatusReserved).
		Count(&claims).Error)
	assert.EqualValues(t, 1, claims)
	assert.Equal(t, 1, successes)
}

func TestCampaignEmailLimitIncludesLiveReservations(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("email-limit", 100)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	createCampaignTestTopUp(
		t,
		User{Username: "email-user-a", Email: " Shared@Example.COM "},
		campaign,
		"email-trade-a",
		now,
	)

	user := User{Username: "email-user-b", Email: "shared@example.com", AffCode: "email-user-b"}
	require.NoError(t, DB.Create(&user).Error)
	topUp := TopUp{
		UserId: user.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "email-trade-b", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	err := CreateTopUpWithCampaignReservations(
		&topUp,
		[]CampaignAwardSnapshot{campaignTestSnapshot(campaign)},
		now,
	)
	assert.ErrorIs(t, err, ErrCampaignReservationUnavailable)
}

func TestPerCampaignReservationCountsSameAccountAcrossPackages(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("per-campaign-account", 30)
	campaign.Eligibility = operation_setting.CampaignEligibilityPerCampaign
	campaign.MaxClaimsPerUser = 2
	campaign.MaxClaimsPerEmail = 2
	campaign.MaxClaimsTotal = 60
	campaign.PackageIDs = []string{"advanced", "professional"}
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	user := User{Username: "per-campaign-account", Email: "per-campaign-account@example.com"}
	first := createCampaignTestTopUpForPackage(t, user, campaign, "advanced", "per-campaign-account-1", now)
	userID := first.UserId
	second := TopUp{
		UserId: userID, PackageId: "professional", CreditQuota: 1_000,
		Money: 490, TradeNo: "per-campaign-account-2", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	require.NoError(t, CreateTopUpWithCampaignReservations(&second, []CampaignAwardSnapshot{func() CampaignAwardSnapshot {
		snapshot := campaignTestSnapshot(campaign)
		snapshot.PackageId = "professional"
		return snapshot
	}()}, now))
	third := TopUp{
		UserId: userID, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "per-campaign-account-3", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	assert.ErrorIs(t, CreateTopUpWithCampaignReservations(&third, []CampaignAwardSnapshot{campaignTestSnapshot(campaign)}, now), ErrCampaignReservationUnavailable)
}

func TestPerCampaignReservationCountsSameEmailAcrossPackages(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("per-campaign-email", 30)
	campaign.Eligibility = operation_setting.CampaignEligibilityPerCampaign
	campaign.MaxClaimsPerUser = 1
	campaign.MaxClaimsPerEmail = 2
	campaign.MaxClaimsTotal = 60
	campaign.PackageIDs = []string{"advanced", "professional"}
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	createCampaignTestTopUpForPackage(t, User{Username: "per-campaign-email-a", Email: "shared-per-campaign@example.com"}, campaign, "advanced", "per-campaign-email-1", now)
	createCampaignTestTopUpForPackage(t, User{Username: "per-campaign-email-b", Email: "shared-per-campaign@example.com"}, campaign, "professional", "per-campaign-email-2", now)
	thirdUser := User{Username: "per-campaign-email-c", Email: "shared-per-campaign@example.com", AffCode: "per-campaign-email-c"}
	require.NoError(t, DB.Create(&thirdUser).Error)
	third := TopUp{
		UserId: thirdUser.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "per-campaign-email-3", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	assert.ErrorIs(t, CreateTopUpWithCampaignReservations(&third, []CampaignAwardSnapshot{campaignTestSnapshot(campaign)}, now), ErrCampaignReservationUnavailable)
}

func TestPerCampaignTwoClaimCapacityAndStatsUseThirtyByTwo(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("per-campaign-capacity", 30)
	campaign.Eligibility = operation_setting.CampaignEligibilityPerCampaign
	campaign.MaxClaimsPerUser = 2
	campaign.MaxClaimsPerEmail = 2
	campaign.MaxClaimsTotal = 60
	campaign.PackageIDs = []string{"advanced", "professional"}
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	for index := 0; index < 30; index++ {
		user := User{
			Username: fmt.Sprintf("per-campaign-capacity-%d", index),
			Email:    fmt.Sprintf("per-campaign-capacity-%d@example.com", index),
		}
		createCampaignTestTopUpForPackage(t, user, campaign, "advanced", fmt.Sprintf("per-campaign-capacity-a-%d", index), now)
		userID := user.Id
		var persisted User
		require.NoError(t, DB.Where("username = ?", user.Username).First(&persisted).Error)
		userID = persisted.Id
		second := TopUp{
			UserId: userID, PackageId: "professional", CreditQuota: 1_000,
			Money: 490, TradeNo: fmt.Sprintf("per-campaign-capacity-p-%d", index), PaymentMethod: "wxpay",
			PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
		}
		snapshot := campaignTestSnapshot(campaign)
		snapshot.PackageId = "professional"
		require.NoError(t, CreateTopUpWithCampaignReservations(&second, []CampaignAwardSnapshot{snapshot}, now))
	}

	stats, err := GetPaymentCampaignClaimStats(now)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.True(t, stats[0].ParticipantLimited)
	assert.EqualValues(t, 30, stats[0].ParticipantsTotal)
	assert.EqualValues(t, 60, stats[0].Total)
	assert.EqualValues(t, 60, stats[0].ClaimsReserved)
	assert.Zero(t, stats[0].Remaining)

	extra := User{Username: "per-campaign-capacity-extra", Email: "per-campaign-capacity-extra@example.com", AffCode: "per-campaign-capacity-extra"}
	require.NoError(t, DB.Create(&extra).Error)
	extraTopUp := TopUp{
		UserId: extra.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "per-campaign-capacity-extra", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	assert.ErrorIs(t, CreateTopUpWithCampaignReservations(&extraTopUp, []CampaignAwardSnapshot{campaignTestSnapshot(campaign)}, now), ErrCampaignReservationUnavailable)
}

func TestPerPackageCampaignRejectsSecondReservationForSameAccountAndPackage(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("same-account-package", 30)
	campaign.MaxClaimsPerUser = 4
	campaign.MaxClaimsPerEmail = 4
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	user := User{Username: "same-account-package", Email: "same-account-package@example.com", AffCode: "same-account-package"}
	first := createCampaignTestTopUp(t, user, campaign, "same-account-package-first", now)
	require.NotZero(t, first.Id)

	second := TopUp{
		UserId: first.UserId, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "same-account-package-second", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	err := CreateTopUpWithCampaignReservations(
		&second,
		[]CampaignAwardSnapshot{campaignTestSnapshot(campaign)},
		now,
	)

	assert.ErrorIs(t, err, ErrCampaignReservationUnavailable)
	var secondCount int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", second.TradeNo).Count(&secondCount).Error)
	assert.Zero(t, secondCount)
}

func TestCampaignEmailLimitSurvivesOwnerEmailChange(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("email-change-limit", 100)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	first := createCampaignTestTopUp(
		t,
		User{Username: "email-change-first", Email: "claimed@example.com"},
		campaign,
		"email-change-first-trade",
		now,
	)
	require.NoError(t, CompleteEpayTopUp(first.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))
	require.NoError(t, DB.Model(&User{}).Where("id = ?", first.UserId).Update("email", "changed@example.com").Error)

	secondUser := User{
		Username: "email-change-second",
		Email:    "claimed@example.com",
		AffCode:  "email-change-second",
	}
	require.NoError(t, DB.Create(&secondUser).Error)
	second := TopUp{
		UserId: secondUser.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "email-change-second-trade", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	err := CreateTopUpWithCampaignReservations(
		&second,
		[]CampaignAwardSnapshot{campaignTestSnapshot(campaign)},
		now,
	)
	assert.ErrorIs(t, err, ErrCampaignReservationUnavailable)
}

func TestExpiredReservationCanBeReusedAndLatePayerGetsRegularCreditOnlyWhenFull(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("expiry-reuse", 1)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	first := createCampaignTestTopUp(
		t,
		User{Username: "expired-first", Email: "expired-first@example.com"},
		campaign,
		"expired-first-trade",
		now-181,
	)
	second := createCampaignTestTopUp(
		t,
		User{Username: "expired-second", Email: "expired-second@example.com"},
		campaign,
		"expired-second-trade",
		now,
	)

	var firstClaim PaymentCampaignClaim
	require.NoError(t, DB.Where("topup_id = ?", first.Id).First(&firstClaim).Error)
	assert.Equal(t, CampaignClaimStatusReleased, firstClaim.Status)
	var secondClaim PaymentCampaignClaim
	require.NoError(t, DB.Where("topup_id = ?", second.Id).First(&secondClaim).Error)
	assert.Equal(t, CampaignClaimStatusReserved, secondClaim.Status)

	require.NoError(t, CompleteEpayTopUp(first.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))
	var completed TopUp
	require.NoError(t, DB.First(&completed, first.Id).Error)
	assert.Zero(t, completed.BonusCreditQuota)
	var bonusCount int64
	require.NoError(t, DB.Model(&BonusBalance{}).Where("topup_id = ?", first.Id).Count(&bonusCount).Error)
	assert.Zero(t, bonusCount)
	var user User
	require.NoError(t, DB.First(&user, first.UserId).Error)
	assert.Equal(t, 1_000, user.Quota)
}

func TestExpiredReservationNoCapacityGetsRegularOnly(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("expiry-reclaim", 1)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	topUp := createCampaignTestTopUp(
		t,
		User{Username: "reclaim-user", Email: "reclaim@example.com"},
		campaign,
		"reclaim-trade",
		now-181,
	)

	require.NoError(t, CompleteEpayTopUp(topUp.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))
	var completed TopUp
	require.NoError(t, DB.First(&completed, topUp.Id).Error)
	assert.Zero(t, completed.BonusCreditQuota)
}

func TestCampaignCloseHonorsLiveReservationButRejectsExpiredReclaim(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("close-contract", 2)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	live := createCampaignTestTopUp(
		t,
		User{Username: "close-live", Email: "close-live@example.com"},
		campaign,
		"close-live-trade",
		now,
	)
	expired := createCampaignTestTopUp(
		t,
		User{Username: "close-expired", Email: "close-expired@example.com"},
		campaign,
		"close-expired-trade",
		now-181,
	)
	operation_setting.GetPaymentSetting().Campaigns[0].Enabled = false

	require.NoError(t, CompleteEpayTopUp(live.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))
	require.NoError(t, CompleteEpayTopUp(expired.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))
	var liveResult, expiredResult TopUp
	require.NoError(t, DB.First(&liveResult, live.Id).Error)
	require.NoError(t, DB.First(&expiredResult, expired.Id).Error)
	assert.EqualValues(t, 280, liveResult.BonusCreditQuota)
	assert.Zero(t, expiredResult.BonusCreditQuota)
}

func TestCampaignStatsSeparateAwardedReservedAndRemaining(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("stats-campaign", 3)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	awarded := createCampaignTestTopUp(
		t,
		User{Username: "stats-awarded", Email: "stats-awarded@example.com"},
		campaign,
		"stats-awarded-trade",
		now,
	)
	createCampaignTestTopUp(
		t,
		User{Username: "stats-reserved", Email: "stats-reserved@example.com"},
		campaign,
		"stats-reserved-trade",
		now,
	)
	require.NoError(t, CompleteEpayTopUp(awarded.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))

	stats, err := GetPaymentCampaignClaimStats(now)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.True(t, stats[0].ParticipantLimited)
	assert.EqualValues(t, 3, stats[0].ParticipantsTotal)
	assert.EqualValues(t, 1, stats[0].ParticipantsAdmitted)
	assert.EqualValues(t, 1, stats[0].ParticipantsReserved)
	assert.EqualValues(t, 1, stats[0].ParticipantsRemaining)
	assert.EqualValues(t, 1, stats[0].ClaimsAwarded)
	assert.EqualValues(t, 1, stats[0].ClaimsReserved)
}

func TestCancelledTopUpReleasesCampaignReservation(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("cancel-release", 1)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	topUp := createCampaignTestTopUp(
		t,
		User{Username: "cancel-user", Email: "cancel@example.com"},
		campaign,
		"cancel-trade",
		now,
	)

	require.NoError(t, UpdatePendingTopUpStatus(topUp.TradeNo, PaymentProviderEpay, common.TopUpStatusFailed))
	var claim PaymentCampaignClaim
	require.NoError(t, DB.Where("topup_id = ?", topUp.Id).First(&claim).Error)
	assert.Equal(t, CampaignClaimStatusReleased, claim.Status)
	assert.Nil(t, claim.ClaimKey)
	assert.Nil(t, claim.EmailClaimKey)
	assert.Nil(t, claim.CampaignSlotKey)
}

func TestCampaignReservationFailureRollsBackTopUp(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("rollback-campaign", 1)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	createCampaignTestTopUp(
		t,
		User{Username: "rollback-first", Email: "rollback-first@example.com"},
		campaign,
		"rollback-first-trade",
		now,
	)
	user := User{Username: "rollback-second", Email: "rollback-second@example.com", AffCode: "rollback-second"}
	require.NoError(t, DB.Create(&user).Error)
	topUp := TopUp{
		UserId: user.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "rollback-second-trade", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
	}
	err := CreateTopUpWithCampaignReservations(&topUp, []CampaignAwardSnapshot{campaignTestSnapshot(campaign)}, now)
	require.True(t, errors.Is(err, ErrCampaignReservationUnavailable))
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", topUp.TradeNo).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCompleteEpayTopUpCreatesIndependentExpiringBonus(t *testing.T) {
	setupEpayTopupTestDB(t)
	user := User{Username: "campaign-user"}
	require.NoError(t, DB.Create(&user).Error)

	snapshot := CampaignAwardSnapshot{
		CampaignId:     "launch-first-topup-30",
		CampaignName:   "Launch first top-up",
		Eligibility:    operation_setting.CampaignEligibilityPerCampaign,
		MaxClaims:      1,
		PackageId:      "advanced",
		BonusQuota:     280,
		BonusAmountCNY: "28.00",
		ValidDays:      45,
	}
	topup := TopUp{
		UserId: user.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "campaign-topup", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
		CampaignSnapshot: EncodeCampaignAwardSnapshots([]CampaignAwardSnapshot{snapshot}),
	}
	require.NoError(t, DB.Create(&topup).Error)
	require.NoError(t, CompleteEpayTopUp(topup.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))

	var updated TopUp
	require.NoError(t, DB.First(&updated, topup.Id).Error)
	assert.EqualValues(t, 280, updated.BonusCreditQuota)
	var bonus BonusBalance
	require.NoError(t, DB.Where("topup_id = ?", topup.Id).First(&bonus).Error)
	assert.EqualValues(t, 280, bonus.AmountTotal)
	assert.InDelta(t, time.Now().Add(45*24*time.Hour).Unix(), bonus.ExpiresAt, 3)

	// Repeated callbacks cannot credit either regular or bonus balance twice.
	require.NoError(t, CompleteEpayTopUp(topup.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))
	var count int64
	require.NoError(t, DB.Model(&BonusBalance{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestCompleteEpayTopUpConcurrentCallbacksCreditExactlyOnce(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("concurrent-callback", 1)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	topUp := createCampaignTestTopUp(
		t,
		User{Username: "concurrent-callback-user", Email: "callback@example.com"},
		campaign,
		"concurrent-callback-trade",
		now,
	)

	start := make(chan struct{})
	results := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for index := 0; index < 2; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			results <- CompleteEpayTopUp(
				topUp.TradeNo,
				"wxpay",
				decimal.NewFromInt(98),
				"127.0.0.1",
			)
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}

	var user User
	require.NoError(t, DB.First(&user, topUp.UserId).Error)
	assert.Equal(t, 1_000, user.Quota)
	var completed TopUp
	require.NoError(t, DB.First(&completed, topUp.Id).Error)
	assert.EqualValues(t, 280, completed.BonusCreditQuota)
	var bonusCount, claimCount int64
	require.NoError(t, DB.Model(&BonusBalance{}).Where("topup_id = ?", topUp.Id).Count(&bonusCount).Error)
	require.NoError(t, DB.Model(&PaymentCampaignClaim{}).
		Where("topup_id = ? AND status = ?", topUp.Id, CampaignClaimStatusAwarded).
		Count(&claimCount).Error)
	assert.EqualValues(t, 1, bonusCount)
	assert.EqualValues(t, 1, claimCount)
}

func TestCompleteEpayTopUpRollsBackAfterBonusInsertFailure(t *testing.T) {
	setupEpayTopupTestDB(t)
	campaign := campaignTestRule("callback-rollback", 1)
	useTestCampaign(t, campaign)
	now := common.GetTimestamp()
	topUp := createCampaignTestTopUp(
		t,
		User{Username: "callback-rollback-user", Email: "rollback@example.com"},
		campaign,
		"callback-rollback-trade",
		now,
	)
	require.NoError(t, DB.Exec(`
		CREATE TRIGGER reject_campaign_bonus
		BEFORE INSERT ON bonus_balances
		BEGIN
			SELECT RAISE(ABORT, 'injected bonus insert failure');
		END
	`).Error)

	err := CompleteEpayTopUp(topUp.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1")
	require.Error(t, err)

	var unchangedTopUp TopUp
	require.NoError(t, DB.First(&unchangedTopUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, unchangedTopUp.Status)
	assert.Zero(t, unchangedTopUp.BonusCreditQuota)
	var user User
	require.NoError(t, DB.First(&user, topUp.UserId).Error)
	assert.Zero(t, user.Quota)
	var claim PaymentCampaignClaim
	require.NoError(t, DB.Where("topup_id = ?", topUp.Id).First(&claim).Error)
	assert.Equal(t, CampaignClaimStatusReserved, claim.Status)
	assert.Zero(t, claim.AwardedAt)
	var bonusCount int64
	require.NoError(t, DB.Model(&BonusBalance{}).Where("topup_id = ?", topUp.Id).Count(&bonusCount).Error)
	assert.Zero(t, bonusCount)
}

func TestWalletConsumesEarliestBonusThenRegularAndRestoresSources(t *testing.T) {
	setupEpayTopupTestDB(t)
	user := User{Username: "wallet-user", Quota: 1_000}
	require.NoError(t, DB.Create(&user).Error)
	now := common.GetTimestamp()
	later := BonusBalance{UserId: user.Id, CampaignId: "later", TopUpId: 1, AmountTotal: 100, ExpiresAt: now + 200}
	sooner := BonusBalance{UserId: user.Id, CampaignId: "sooner", TopUpId: 2, AmountTotal: 100, ExpiresAt: now + 100}
	require.NoError(t, DB.Create(&later).Error)
	require.NoError(t, DB.Create(&sooner).Error)

	result, err := PreConsumeWalletFunds("wallet-order", user.Id, 250)
	require.NoError(t, err)
	assert.EqualValues(t, 200, result.BonusQuota)
	assert.EqualValues(t, 50, result.WalletQuota)

	require.NoError(t, DB.First(&sooner, sooner.Id).Error)
	require.NoError(t, DB.First(&later, later.Id).Error)
	assert.EqualValues(t, 100, sooner.AmountUsed)
	assert.EqualValues(t, 100, later.AmountUsed)
	var updatedUser User
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 950, updatedUser.Quota)

	require.NoError(t, AdjustWalletConsumption("wallet-order", user.Id, -70))
	require.NoError(t, DB.First(&later, later.Id).Error)
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	assert.EqualValues(t, 80, later.AmountUsed)
	assert.Equal(t, 1_000, updatedUser.Quota)

	require.NoError(t, RefundWalletConsumption("wallet-order", user.Id))
	require.NoError(t, DB.First(&sooner, sooner.Id).Error)
	require.NoError(t, DB.First(&later, later.Id).Error)
	assert.Zero(t, sooner.AmountUsed)
	assert.Zero(t, later.AmountUsed)
}

func TestExpiredBonusIsNotSpendable(t *testing.T) {
	setupEpayTopupTestDB(t)
	user := User{Username: "expired-wallet-user", Quota: 50}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&BonusBalance{
		UserId: user.Id, CampaignId: "expired", TopUpId: 3,
		AmountTotal: 100, ExpiresAt: common.GetTimestamp() - 1,
	}).Error)

	spendable, err := GetUserSpendableQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 50, spendable)
	_, err = PreConsumeWalletFunds("expired-order", user.Id, 51)
	assert.Error(t, err)
}

func TestGetBonusBalanceSummariesUsesSharedNowAndIgnoresInvalidLots(t *testing.T) {
	setupEpayTopupTestDB(t)
	now := int64(1_000)
	first := User{Username: "summary-first", AffCode: "summary-first", Quota: 300}
	second := User{Username: "summary-second", AffCode: "summary-second", Quota: 400}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)
	require.NoError(t, DB.Create([]BonusBalance{
		{UserId: first.Id, CampaignId: "first-valid", TopUpId: 1, Status: BonusBalanceStatusActive, AmountTotal: 100, AmountUsed: 25, ExpiresAt: now + 100},
		{UserId: first.Id, CampaignId: "second-valid", TopUpId: 2, Status: BonusBalanceStatusActive, AmountTotal: 50, AmountUsed: 0, ExpiresAt: now + 50},
		{UserId: first.Id, CampaignId: "boundary", TopUpId: 3, Status: BonusBalanceStatusActive, AmountTotal: 999, AmountUsed: 0, ExpiresAt: now},
		{UserId: first.Id, CampaignId: "used-up", TopUpId: 4, Status: BonusBalanceStatusActive, AmountTotal: 999, AmountUsed: 999, ExpiresAt: now + 10},
		{UserId: first.Id, CampaignId: "expired", TopUpId: 5, Status: BonusBalanceStatusExpired, AmountTotal: 999, AmountUsed: 0, ExpiresAt: now + 1},
		{UserId: second.Id, CampaignId: "third-valid", TopUpId: 6, Status: BonusBalanceStatusActive, AmountTotal: 20, AmountUsed: 5, ExpiresAt: now + 200},
	}).Error)

	summaries, err := GetBonusBalanceSummaries([]int{first.Id, second.Id}, now)
	require.NoError(t, err)
	assert.Equal(t, BonusBalanceSummary{ActiveQuota: 125, NearestExpiresAt: now + 50}, summaries[first.Id])
	assert.Equal(t, BonusBalanceSummary{ActiveQuota: 15, NearestExpiresAt: now + 200}, summaries[second.Id])
	assert.NotContains(t, summaries, 99999)

	empty, err := GetBonusBalanceSummaries(nil, now)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
