package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestTopUpProductBrandUsesTheChinesePublicBrand(t *testing.T) {
	assert.Equal(t, common.PublicBrandZhCN+" 100 元额度", buildEpayProductName("100 元额度"))
}
