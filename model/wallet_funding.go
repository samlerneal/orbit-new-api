package model

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	WalletConsumeStatusConsumed = "consumed"
	WalletConsumeStatusRefunded = "refunded"
)

type BonusConsumeAllocation struct {
	BonusBalanceId int   `json:"bonus_balance_id"`
	Amount         int64 `json:"amount"`
}

// WalletConsumeRecord makes wallet pre-consume, settlement and refund
// idempotent while preserving the split between expiring and regular balance.
type WalletConsumeRecord struct {
	Id              int    `json:"id"`
	RequestId       string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId          int    `json:"user_id" gorm:"index"`
	BonusQuota      int64  `json:"bonus_quota" gorm:"type:bigint;not null;default:0"`
	WalletQuota     int64  `json:"wallet_quota" gorm:"type:bigint;not null;default:0"`
	AllocationsJson string `json:"-" gorm:"type:text"`
	Status          string `json:"status" gorm:"type:varchar(24);index"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint;index"`
}

func (record *WalletConsumeRecord) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.Status == "" {
		record.Status = WalletConsumeStatusConsumed
	}
	return nil
}

func (record *WalletConsumeRecord) BeforeUpdate(tx *gorm.DB) error {
	record.UpdatedAt = common.GetTimestamp()
	return nil
}

func (record *WalletConsumeRecord) allocations() ([]BonusConsumeAllocation, error) {
	if record.AllocationsJson == "" {
		return nil, nil
	}
	var allocations []BonusConsumeAllocation
	err := json.Unmarshal([]byte(record.AllocationsJson), &allocations)
	return allocations, err
}

func (record *WalletConsumeRecord) setAllocations(allocations []BonusConsumeAllocation) error {
	if len(allocations) == 0 {
		record.AllocationsJson = ""
		return nil
	}
	encoded, err := json.Marshal(allocations)
	if err != nil {
		return err
	}
	record.AllocationsJson = string(encoded)
	return nil
}

type WalletConsumeResult struct {
	BonusQuota  int64
	WalletQuota int64
}

func GetUserSpendableQuota(userId int) (int, error) {
	regularQuota, err := GetUserQuota(userId, false)
	if err != nil {
		return 0, err
	}
	summary, err := GetBonusBalanceSummary(userId)
	if err != nil {
		return 0, err
	}
	total := int64(regularQuota) + summary.ActiveQuota
	if total > int64(common.MaxQuota) {
		return common.MaxQuota, nil
	}
	return int(total), nil
}

func consumeBonusLotsTx(tx *gorm.DB, userId int, amount int64, allocations []BonusConsumeAllocation) (int64, []BonusConsumeAllocation, error) {
	if amount <= 0 {
		return 0, allocations, nil
	}
	now := common.GetTimestamp()
	var balances []BonusBalance
	if err := lockForUpdate(tx).
		Where("user_id = ? AND status = ? AND expires_at > ? AND amount_total > amount_used", userId, BonusBalanceStatusActive, now).
		Order("expires_at asc, id asc").
		Find(&balances).Error; err != nil {
		return 0, allocations, err
	}
	remaining := amount
	consumed := int64(0)
	for _, balance := range balances {
		available := balance.AmountTotal - balance.AmountUsed
		if available <= 0 {
			continue
		}
		take := available
		if take > remaining {
			take = remaining
		}
		balance.AmountUsed += take
		if err := tx.Save(&balance).Error; err != nil {
			return consumed, allocations, err
		}
		allocations = append(allocations, BonusConsumeAllocation{BonusBalanceId: balance.Id, Amount: take})
		consumed += take
		remaining -= take
		if remaining == 0 {
			break
		}
	}
	return consumed, allocations, nil
}

func consumeWalletTx(tx *gorm.DB, record *WalletConsumeRecord, amount int64) (WalletConsumeResult, error) {
	if amount <= 0 {
		return WalletConsumeResult{}, nil
	}
	var user User
	if err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", record.UserId).First(&user).Error; err != nil {
		return WalletConsumeResult{}, err
	}
	allocations, err := record.allocations()
	if err != nil {
		return WalletConsumeResult{}, err
	}
	bonusConsumed, allocations, err := consumeBonusLotsTx(tx, record.UserId, amount, allocations)
	if err != nil {
		return WalletConsumeResult{}, err
	}
	walletConsumed := amount - bonusConsumed
	if walletConsumed > int64(user.Quota) {
		return WalletConsumeResult{}, fmt.Errorf("insufficient wallet quota")
	}
	if walletConsumed > 0 {
		result := tx.Model(&User{}).
			Where("id = ? AND quota >= ?", record.UserId, walletConsumed).
			Update("quota", gorm.Expr("quota - ?", walletConsumed))
		if result.Error != nil {
			return WalletConsumeResult{}, result.Error
		}
		if result.RowsAffected != 1 {
			return WalletConsumeResult{}, fmt.Errorf("insufficient wallet quota")
		}
	}
	record.BonusQuota += bonusConsumed
	record.WalletQuota += walletConsumed
	if err := record.setAllocations(allocations); err != nil {
		return WalletConsumeResult{}, err
	}
	return WalletConsumeResult{BonusQuota: bonusConsumed, WalletQuota: walletConsumed}, nil
}

func PreConsumeWalletFunds(requestId string, userId int, amount int) (WalletConsumeResult, error) {
	if requestId == "" || userId <= 0 || amount < 0 {
		return WalletConsumeResult{}, errors.New("invalid wallet consume request")
	}
	if amount == 0 {
		return WalletConsumeResult{}, nil
	}
	result := WalletConsumeResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing WalletConsumeRecord
		query := lockForUpdate(tx).Where("request_id = ?", requestId).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if existing.Status == WalletConsumeStatusRefunded {
				return errors.New("wallet consume already refunded")
			}
			result = WalletConsumeResult{BonusQuota: existing.BonusQuota, WalletQuota: existing.WalletQuota}
			return nil
		}
		record := &WalletConsumeRecord{RequestId: requestId, UserId: userId}
		consumed, err := consumeWalletTx(tx, record, int64(amount))
		if err != nil {
			return err
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		result = consumed
		return nil
	})
	if err == nil && result.WalletQuota > 0 {
		if cacheErr := cacheDecrUserQuota(userId, result.WalletQuota); cacheErr != nil {
			common.SysLog("failed to decrease wallet quota cache: " + cacheErr.Error())
		}
	}
	return result, err
}

func restoreWalletTx(tx *gorm.DB, record *WalletConsumeRecord, amount int64) (WalletConsumeResult, error) {
	if amount <= 0 {
		return WalletConsumeResult{}, nil
	}
	available := record.BonusQuota + record.WalletQuota
	if amount > available {
		return WalletConsumeResult{}, errors.New("wallet refund exceeds consumed amount")
	}
	result := WalletConsumeResult{}
	walletRestore := amount
	if walletRestore > record.WalletQuota {
		walletRestore = record.WalletQuota
	}
	if walletRestore > 0 {
		if err := tx.Model(&User{}).Where("id = ?", record.UserId).
			Update("quota", gorm.Expr("quota + ?", walletRestore)).Error; err != nil {
			return result, err
		}
		record.WalletQuota -= walletRestore
		result.WalletQuota = walletRestore
	}
	bonusRestore := amount - walletRestore
	allocations, err := record.allocations()
	if err != nil {
		return result, err
	}
	for index := len(allocations) - 1; index >= 0 && bonusRestore > 0; index-- {
		allocation := &allocations[index]
		restore := allocation.Amount
		if restore > bonusRestore {
			restore = bonusRestore
		}
		update := tx.Model(&BonusBalance{}).
			Where("id = ? AND amount_used >= ?", allocation.BonusBalanceId, restore).
			Update("amount_used", gorm.Expr("amount_used - ?", restore))
		if update.Error != nil {
			return result, update.Error
		}
		if update.RowsAffected != 1 {
			return result, errors.New("bonus balance refund conflict")
		}
		allocation.Amount -= restore
		record.BonusQuota -= restore
		result.BonusQuota += restore
		bonusRestore -= restore
	}
	filtered := allocations[:0]
	for _, allocation := range allocations {
		if allocation.Amount > 0 {
			filtered = append(filtered, allocation)
		}
	}
	if err := record.setAllocations(filtered); err != nil {
		return result, err
	}
	return result, nil
}

// AdjustWalletConsumption applies a post-consume delta. Positive values consume
// more using earliest-expiry bonus first; negative values restore regular quota
// first so the remaining consumption still honors bonus-first ordering.
func AdjustWalletConsumption(requestId string, userId int, delta int) error {
	if requestId == "" || userId <= 0 || delta == 0 {
		return nil
	}
	cacheDelta := int64(0)
	err := DB.Transaction(func(tx *gorm.DB) error {
		var record WalletConsumeRecord
		query := lockForUpdate(tx).Where("request_id = ?", requestId).Limit(1).Find(&record)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected == 0 {
			if delta < 0 {
				return errors.New("wallet consume record not found")
			}
			record = WalletConsumeRecord{RequestId: requestId, UserId: userId}
			result, err := consumeWalletTx(tx, &record, int64(delta))
			if err != nil {
				return err
			}
			cacheDelta -= result.WalletQuota
			return tx.Create(&record).Error
		}
		if record.UserId != userId || record.Status != WalletConsumeStatusConsumed {
			return errors.New("wallet consume record is not adjustable")
		}
		if delta > 0 {
			result, err := consumeWalletTx(tx, &record, int64(delta))
			if err != nil {
				return err
			}
			cacheDelta -= result.WalletQuota
		} else {
			result, err := restoreWalletTx(tx, &record, int64(-delta))
			if err != nil {
				return err
			}
			cacheDelta += result.WalletQuota
		}
		return tx.Save(&record).Error
	})
	if err == nil && cacheDelta != 0 {
		if cacheErr := cacheIncrUserQuota(userId, cacheDelta); cacheErr != nil {
			common.SysLog("failed to adjust wallet quota cache: " + cacheErr.Error())
		}
	}
	return err
}

func RefundWalletConsumption(requestId string, userId int) error {
	if requestId == "" || userId <= 0 {
		return errors.New("invalid wallet refund request")
	}
	regularRestored := int64(0)
	err := DB.Transaction(func(tx *gorm.DB) error {
		var record WalletConsumeRecord
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&record).Error; err != nil {
			return err
		}
		if record.UserId != userId {
			return errors.New("wallet consume record user mismatch")
		}
		if record.Status == WalletConsumeStatusRefunded {
			return nil
		}
		result, err := restoreWalletTx(tx, &record, record.BonusQuota+record.WalletQuota)
		if err != nil {
			return err
		}
		regularRestored = result.WalletQuota
		record.Status = WalletConsumeStatusRefunded
		return tx.Save(&record).Error
	})
	if err == nil && regularRestored > 0 {
		if cacheErr := cacheIncrUserQuota(userId, regularRestored); cacheErr != nil {
			common.SysLog("failed to refund wallet quota cache: " + cacheErr.Error())
		}
	}
	return err
}
