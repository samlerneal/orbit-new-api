package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()

	payMethods := make([]map[string]string, 0, 1)
	if complianceConfirmed {
		for _, method := range operation_setting.PayMethods {
			if method["type"] == "wxpay" {
				payMethods = append(payMethods, method)
				break
			}
		}
	}

	userId := c.GetInt("id")
	topupPackages := make([]gin.H, 0)
	visibleCampaigns := make(map[string]gin.H)
	campaignParticipantStats := make(map[string]model.PaymentCampaignClaimStats)
	allStats, statsErr := model.GetPaymentCampaignClaimStats(time.Now().Unix())
	if statsErr == nil {
		for _, stat := range allStats {
			campaignParticipantStats[stat.CampaignId] = stat
		}
	}
	for _, packageOption := range operation_setting.GetTopupPackages() {
		resolvedPackage, ok := operation_setting.ResolveTopupPackage(packageOption.ID)
		if ok {
			offers, err := model.ResolveCampaignOffers(userId, resolvedPackage, time.Now().Unix())
			if err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("读取充值活动失败 user_id=%d package_id=%s error=%q", userId, resolvedPackage.ID, err.Error()))
				offers = nil
			}
			campaignBadges := make([]gin.H, 0, len(offers))
			displayCredit := resolvedPackage.CreditAmount
			for _, offer := range offers {
				displayCredit += offer.BonusAmount
				campaignBadges = append(campaignBadges, gin.H{
					"campaign_id": offer.Campaign.ID,
					"text":        offer.Campaign.BadgeText,
					"valid_days":  offer.Campaign.ValidDays,
				})
				grossBonus := offer.TotalAmount - resolvedPackage.PayAmount
				campaignInfo, exists := visibleCampaigns[offer.Campaign.ID]
				if !exists || grossBonus > campaignInfo["max_bonus"].(float64) {
					participantStat := campaignParticipantStats[offer.Campaign.ID]
					visibleCampaigns[offer.Campaign.ID] = gin.H{
						"id":                    offer.Campaign.ID,
						"title":                 offer.Campaign.BannerTitle,
						"description":           offer.Campaign.BannerText,
						"badge_text":            offer.Campaign.BadgeText,
						"max_bonus":             grossBonus,
						"valid_days":            offer.Campaign.ValidDays,
						"priority":              offer.Campaign.Priority,
						"participant_limited":   participantStat.ParticipantLimited,
						"participant_total":     participantStat.ParticipantsTotal,
						"participant_remaining": participantStat.ParticipantsRemaining,
						"cumulative_max_bonus":  computeCumulativeMaxBonus(offer.Campaign, operation_setting.GetTopupPackages()),
					}
				}
			}
			topupPackages = append(topupPackages, gin.H{
				"id":                    resolvedPackage.ID,
				"name":                  resolvedPackage.Name,
				"description":           resolvedPackage.Description,
				"tag":                   resolvedPackage.Tag,
				"pay_amount":            resolvedPackage.PayAmount,
				"credit_amount":         resolvedPackage.CreditAmount,
				"display_credit_amount": displayCredit,
				"bonus_amount":          displayCredit - resolvedPackage.CreditAmount,
				"campaign_badges":       campaignBadges,
				"sort_order":            resolvedPackage.SortOrder,
				"selling_points":        resolvedPackage.SellingPoints,
				"footer_note":           resolvedPackage.FooterNote,
				"visual_style":          resolvedPackage.VisualStyle,
			})
		}
	}
	sort.SliceStable(topupPackages, func(i, j int) bool {
		return topupPackages[i]["sort_order"].(int) < topupPackages[j]["sort_order"].(int)
	})
	campaigns := make([]gin.H, 0, len(visibleCampaigns))
	for _, campaign := range visibleCampaigns {
		campaigns = append(campaigns, campaign)
	}
	sort.SliceStable(campaigns, func(i, j int) bool {
		return campaigns[i]["priority"].(int) > campaigns[j]["priority"].(int)
	})
	bonusSummary, err := model.GetBonusBalanceSummary(userId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("读取活动余额失败 user_id=%d error=%q", userId, err.Error()))
	}
	enableWeChatTopup := isEpayTopUpEnabled() && len(payMethods) == 1

	data := gin.H{
		"enable_online_topup":              enableWeChatTopup,
		"enable_stripe_topup":              false,
		"enable_creem_topup":               false,
		"enable_waffo_topup":               false,
		"enable_waffo_pancake_topup":       false,
		"enable_redemption":                complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"refund_notice_version":            operation_setting.CurrentRefundNoticeVersion,
		"support_contacts":                 operation_setting.GetSupportContacts(),
		"waffo_pay_methods":                nil,
		"creem_products":                   nil,
		"pay_methods":                      payMethods,
		"topup_packages":                   topupPackages,
		"campaigns":                        campaigns,
		"promotion_enabled":                len(campaigns) > 0,
		"bonus_balance_quota":              bonusSummary.ActiveQuota,
		"bonus_nearest_expires_at":         bonusSummary.NearestExpiresAt,
		"min_topup":                        operation_setting.MinTopUp,
		"stripe_min_topup":                 setting.StripeMinTopUp,
		"waffo_min_topup":                  setting.WaffoMinTopUp,
		"waffo_pancake_min_topup":          setting.WaffoPancakeMinTopUp,
		"amount_options":                   operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                         operation_setting.GetPaymentSetting().AmountDiscount,
		"topup_link":                       common.TopUpLink,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	PackageID            string `json:"package_id"`
	PaymentMethod        string `json:"payment_method"`
	RefundNoticeAccepted bool   `json:"refund_notice_accepted"`
	RefundNoticeVersion  string `json:"refund_notice_version"`
	RefundNoticeLanguage string `json:"refund_notice_language"`
}

type AmountRequest struct {
	Amount int64 `json:"amount"`
}

var supportedRefundNoticeLanguages = map[string]struct{}{
	"en":   {},
	"fr":   {},
	"ja":   {},
	"ru":   {},
	"vi":   {},
	"zhCN": {},
	"zhTW": {},
}

func validateRefundNoticeAcceptance(req EpayRequest) error {
	if !req.RefundNoticeAccepted {
		return errors.New("请阅读并同意当前充值退款说明")
	}
	if strings.TrimSpace(req.RefundNoticeVersion) != operation_setting.CurrentRefundNoticeVersion {
		return errors.New("充值退款说明已更新，请刷新页面后重新确认")
	}
	language := strings.TrimSpace(req.RefundNoticeLanguage)
	if _, ok := supportedRefundNoticeLanguages[language]; !ok {
		return errors.New("充值退款说明语言无效，请刷新页面后重试")
	}
	return nil
}

func recordRefundNoticeAcceptance(topUp *model.TopUp, req EpayRequest, acceptedAt int64) {
	topUp.RefundNoticeVersion = operation_setting.CurrentRefundNoticeVersion
	topUp.RefundNoticeAcceptedAt = acceptedAt
	topUp.RefundNoticeLanguage = strings.TrimSpace(req.RefundNoticeLanguage)
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if err := validateRefundNoticeAcceptance(req); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	packageOption, ok := operation_setting.ResolveTopupPackage(req.PackageID)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值档位不存在"})
		return
	}
	if req.PaymentMethod != "wxpay" || !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}
	if !operation_setting.IsPaymentComplianceConfirmed() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付功能暂不可用"})
		return
	}

	creditAmount := decimal.NewFromFloat(packageOption.CreditAmount)
	exchangeRate := decimal.NewFromFloat(operation_setting.USDExchangeRate)
	if creditAmount.LessThanOrEqual(decimal.Zero) || exchangeRate.LessThanOrEqual(decimal.Zero) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值档位配置错误"})
		return
	}
	creditUSD := creditAmount.Div(exchangeRate)
	creditQuota, quotaClamp := common.QuotaFromDecimalChecked(creditUSD.Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if creditQuota <= 0 || quotaClamp != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值额度配置错误"})
		return
	}

	id := c.GetInt("id")
	now := time.Now().Unix()
	payMoney := decimal.NewFromFloat(packageOption.PayAmount).Round(2)
	campaignSnapshots, err := model.BuildCampaignAwardSnapshots(id, packageOption, now)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("计算充值活动失败 user_id=%d package_id=%s error=%q", id, req.PackageID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值活动暂不可用，请稍后重试"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/wallet?show_history=true"))
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           "Orbit " + packageOption.Name,
		Money:          payMoney.StringFixed(2),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s package_id=%s money=%s error=%q", id, tradeNo, req.PaymentMethod, req.PackageID, payMoney.StringFixed(2), err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	topUp := &model.TopUp{
		UserId:           id,
		PackageId:        packageOption.ID,
		Amount:           creditUSD.Round(0).IntPart(),
		CreditQuota:      int64(creditQuota),
		CampaignSnapshot: model.EncodeCampaignAwardSnapshots(campaignSnapshots),
		Money:            payMoney.InexactFloat64(),
		TradeNo:          tradeNo,
		PaymentMethod:    req.PaymentMethod,
		PaymentProvider:  model.PaymentProviderEpay,
		CreateTime:       now,
		Status:           common.TopUpStatusPending,
	}
	recordRefundNoticeAcceptance(topUp, req, now)
	err = model.CreateTopUpWithCampaignReservations(topUp, campaignSnapshots, now)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s package_id=%s error=%q", id, tradeNo, req.PaymentMethod, req.PackageID, err.Error()))
		if errors.Is(err, model.ErrCampaignReservationUnavailable) {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "活动名额刚刚发生变化，请刷新页面后重新确认"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s package_id=%s credit_quota=%d money=%s refund_notice_version=%s refund_notice_language=%s refund_notice_accepted_at=%d uri=%q params=%q", id, tradeNo, req.PaymentMethod, req.PackageID, creditQuota, payMoney.StringFixed(2), topUp.RefundNoticeVersion, topUp.RefundNoticeLanguage, topUp.RefundNoticeAcceptedAt, uri, common.GetJsonString(params)))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(params)))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	client := GetEpayClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err == nil && verifyInfo.VerifyStatus {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签成功 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
	} else {
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, c.ClientIP()))
		}
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	reportedMoney, err := decimal.NewFromString(verifyInfo.Money)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 金额格式无效 trade_no=%s money=%q client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Money, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if err := model.CompleteEpayTopUp(verifyInfo.ServiceTradeNo, verifyInfo.Type, reportedMoney, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 充值订单处理失败 trade_no=%s callback_type=%s money=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, reportedMoney.StringFixed(2), c.ClientIP(), err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if _, err := c.Writer.Write([]byte("success")); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), err.Error()))
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.EnrichTopUpsWithBonusExpiry(topups); err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.EnrichTopUpsWithBonusExpiry(topups); err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
// computeCumulativeMaxBonus calculates the sum of (total_amount - pay_amount)
// across all enabled packages applicable to the campaign. This is the campaign-wide
// advertising ceiling, independent of any user's claim progress.
func computeCumulativeMaxBonus(
	campaign operation_setting.PaymentCampaign,
	packages []operation_setting.TopupPackage,
) int64 {
	var total int64
	for _, packageOption := range packages {
		if !packageOption.Enabled || packageOption.PayAmount <= 0 {
			continue
		}
		if !campaignAppliesToPackageController(campaign, packageOption.ID) {
			continue
		}
		totalAmount, bonusAmount, err := model.ResolveCampaignReward(campaign, packageOption)
		if err != nil || bonusAmount.LessThanOrEqual(decimal.Zero) {
			continue
		}
		grossBonus := totalAmount.Sub(decimal.NewFromFloat(packageOption.PayAmount))
		fen := grossBonus.Mul(decimal.NewFromInt(100)).IntPart()
		total += fen
	}
	return total / 100
}

func campaignAppliesToPackageController(campaign operation_setting.PaymentCampaign, packageId string) bool {
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

func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
