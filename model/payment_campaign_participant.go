package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	CampaignParticipantStatusReserved = "reserved"
	CampaignParticipantStatusAdmitted = "admitted"
	CampaignParticipantStatusReleased = "released"
)

// PaymentCampaignParticipant tracks which accounts have joined a campaign.
// A participant occupies one campaign slot once admitted; reserved
// participants hold a slot temporarily. Released rows retain audit
// metadata while nullable unique keys are cleared so slots and email
// bindings can be reused.
type PaymentCampaignParticipant struct {
	Id                  int     `json:"id"`
	CampaignId          string  `json:"campaign_id" gorm:"type:varchar(64);uniqueIndex:idx_campaign_participant_user,priority:1"`
	UserId              int     `json:"user_id" gorm:"uniqueIndex:idx_campaign_participant_user,priority:2"`
	EmailHash           string  `json:"-" gorm:"type:char(64);index"`
	Status              string  `json:"status" gorm:"type:varchar(24);not null;default:'reserved';index:idx_campaign_participant_status_expiry,priority:1"`
	ParticipantKey      *string `json:"-" gorm:"type:varchar(255);uniqueIndex"`
	EmailParticipantKey *string `json:"-" gorm:"type:varchar(255)"`
	CampaignSlotKey     *string `json:"-" gorm:"type:varchar(160)"`
	ReservedUntil       int64   `json:"reserved_until" gorm:"type:bigint;index:idx_campaign_participant_status_expiry,priority:2"`
	AdmittedAt          int64   `json:"admitted_at" gorm:"type:bigint"`
	ReleasedAt          int64   `json:"released_at" gorm:"type:bigint"`
	CreatedAt           int64   `json:"created_at" gorm:"type:bigint;index"`
	UpdatedAt           int64   `json:"updated_at" gorm:"type:bigint"`
}

func (p *PaymentCampaignParticipant) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if p.CreatedAt == 0 {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = CampaignParticipantStatusReserved
	}
	return nil
}

func ensurePaymentCampaignParticipantIndexes(db *gorm.DB) error {
	indexes := []struct {
		name    string
		column  string
		partial string
	}{
		{name: "idx_campaign_participant_email_key", column: "email_participant_key"},
		{name: "idx_campaign_participant_slot_key", column: "campaign_slot_key"},
	}
	for _, index := range indexes {
		if db.Migrator().HasIndex(&PaymentCampaignParticipant{}, index.name) {
			continue
		}
		sql := "CREATE UNIQUE INDEX " + index.name + " ON payment_campaign_participants (" + index.column + ")"
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}
