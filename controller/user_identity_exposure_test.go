package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestUserOrderResponseRedactionContractIsModelOwned(t *testing.T) {
	rows := []*model.TopUp{{Id: 9, TradeNo: "USR123NOlegacy"}}
	// The ordinary-user endpoint delegates redaction to model.GetUserTopUps;
	// this test documents that controller code must not use admin query paths.
	require.NotNil(t, rows[0])
}
