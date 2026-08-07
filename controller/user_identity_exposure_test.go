package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfUserDataExposesPublicIDButNeverInternalID(t *testing.T) {
	user := &model.User{
		Id:         654321,
		InternalId: 1,
		Username:   "owner",
		Role:       1,
		Status:     1,
		Setting:    `{}`,
	}
	data := buildSelfUserData(user)
	assert.Equal(t, 654321, data["id"])
	_, exposed := data["internal_id"]
	assert.False(t, exposed)

	payload, err := json.Marshal(data)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "internal_id")
}
