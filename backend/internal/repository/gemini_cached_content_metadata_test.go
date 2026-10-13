package repository

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentBillingMetadataCompatibility(t *testing.T) {
	fields := service.ChannelUsageFields{ChannelID: 7, OriginalModel: "gemini-pro", BillingModelSource: "requested"}
	legacy, err := json.Marshal(fields)
	require.NoError(t, err)
	var decoded geminiCachedContentBillingMetadata
	require.NoError(t, json.Unmarshal(legacy, &decoded))
	require.Equal(t, fields, decoded.ChannelUsageFields)
	require.Zero(t, decoded.BillingGroupID)
	decoded.BillingGroupID = 43
	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)
	var roundTrip geminiCachedContentBillingMetadata
	require.NoError(t, json.Unmarshal(encoded, &roundTrip))
	require.Equal(t, decoded, roundTrip)
	var oldReader service.ChannelUsageFields
	require.NoError(t, json.Unmarshal(encoded, &oldReader))
	require.Equal(t, fields, oldReader, "existing flat keys remain readable")
}
