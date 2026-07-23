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
package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func validRefundNoticeRequest() EpayRequest {
	return EpayRequest{
		PackageID:            "experience",
		PaymentMethod:        "wxpay",
		RefundNoticeAccepted: true,
		RefundNoticeVersion:  operation_setting.CurrentRefundNoticeVersion,
		RefundNoticeLanguage: "zhCN",
	}
}

func TestValidateRefundNoticeAcceptanceRequiresExplicitAcceptance(t *testing.T) {
	request := validRefundNoticeRequest()
	request.RefundNoticeAccepted = false

	require.EqualError(t, validateRefundNoticeAcceptance(request), "请阅读并同意当前充值退款说明")
}

func TestValidateRefundNoticeAcceptanceRejectsStaleVersion(t *testing.T) {
	request := validRefundNoticeRequest()
	request.RefundNoticeVersion = "refund-notice-old"

	require.EqualError(t, validateRefundNoticeAcceptance(request), "充值退款说明已更新，请刷新页面后重新确认")
}

func TestValidateRefundNoticeAcceptanceRejectsUnsupportedLanguage(t *testing.T) {
	request := validRefundNoticeRequest()
	request.RefundNoticeLanguage = "unsupported"

	require.EqualError(t, validateRefundNoticeAcceptance(request), "充值退款说明语言无效，请刷新页面后重试")
}

func TestValidateRefundNoticeAcceptanceAllowsSupportedLanguages(t *testing.T) {
	for language := range supportedRefundNoticeLanguages {
		request := validRefundNoticeRequest()
		request.RefundNoticeLanguage = language
		require.NoError(t, validateRefundNoticeAcceptance(request), language)
	}
}

func TestRecordRefundNoticeAcceptanceStoresServerTimeAndCurrentVersion(t *testing.T) {
	request := validRefundNoticeRequest()
	request.RefundNoticeVersion = "  " + operation_setting.CurrentRefundNoticeVersion + "  "
	request.RefundNoticeLanguage = "  zhCN  "
	topUp := &model.TopUp{}

	recordRefundNoticeAcceptance(topUp, request, 1_784_796_400)

	require.Equal(t, operation_setting.CurrentRefundNoticeVersion, topUp.RefundNoticeVersion)
	require.Equal(t, int64(1_784_796_400), topUp.RefundNoticeAcceptedAt)
	require.Equal(t, "zhCN", topUp.RefundNoticeLanguage)
}
