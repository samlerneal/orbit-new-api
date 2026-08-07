package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestWaffoOrdinaryTradeNumberIsOpaque(t *testing.T) {
	tradeNo, err := service.NewWaffoTradeNo("WAFFO-")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(tradeNo, "WAFFO-"))
	require.NotContains(t, tradeNo, "123456789012")
}
