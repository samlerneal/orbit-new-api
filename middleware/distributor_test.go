package middleware

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNormalizeImageStudioRequestReplacesClientSpecification(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/pg/images/generations", strings.NewReader(`{"prompt":"  山水  ","aspect":"landscape"}`))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)

	require.NoError(t, normalizeImageStudioRequest(c))
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	body, err := storage.Bytes()
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-image-2","prompt":"山水","n":1,"size":"1536x1024","quality":"low","response_format":"b64_json","background":"opaque","output_format":"png","stream":false}`, string(body))
	var rewritten struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
	}
	require.NoError(t, common.UnmarshalBodyReusable(c, &rewritten))
	require.Equal(t, "山水", rewritten.Prompt)
	require.Equal(t, "gpt-image-2", rewritten.Model)
}

func TestNormalizeImageStudioRequestMapsOnlyNativePresetSizes(t *testing.T) {
	for _, testCase := range []struct {
		aspect string
		size   string
	}{
		{aspect: "square", size: "1024x1024"},
		{aspect: "landscape", size: "1536x1024"},
	} {
		t.Run(testCase.aspect, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/pg/images/generations", strings.NewReader(fmt.Sprintf(`{"prompt":"safe","aspect":"%s"}`, testCase.aspect)))
			c.Request.Header.Set("Content-Type", gin.MIMEJSON)

			require.NoError(t, normalizeImageStudioRequest(c))
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			body, err := storage.Bytes()
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"model":"gpt-image-2","prompt":"safe","n":1,"size":"%s","quality":"low","response_format":"b64_json","background":"opaque","output_format":"png","stream":false}`, testCase.size), string(body))
		})
	}
}

func TestNormalizeImageStudioRequestRejectsInvalidAspectValues(t *testing.T) {
	for _, body := range []string{
		`{"prompt":"safe"}`,
		`{"prompt":"safe","aspect":""}`,
		`{"prompt":"safe","aspect":null}`,
		`{"prompt":"safe","aspect":1}`,
		`{"prompt":"safe","aspect":"xiaohongshu"}`,
		`{"prompt":"safe","aspect":"Square"}`,
		`{"prompt":"safe","aspect":"wide"}`,
		`{"prompt":"safe","aspect":"portrait"}`,
		`{"prompt":"safe","aspect":"custom"}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/pg/images/generations", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", gin.MIMEJSON)
		require.Error(t, normalizeImageStudioRequest(c), body)
	}
}

func TestNormalizeImageStudioRequestRejectsUnknownFields(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/pg/images/generations", strings.NewReader(`{"prompt":"safe","model":"other"}`))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)

	require.Error(t, normalizeImageStudioRequest(c))
}

func TestNormalizeImageStudioRequestEnforcesUnicodeAndSpecificationBoundaries(t *testing.T) {
	testCases := []struct {
		name     string
		body     string
		accepted bool
	}{
		{name: "empty", body: `{"prompt":"","aspect":"landscape"}`},
		{name: "whitespace", body: `{"prompt":" \t\n ","aspect":"landscape"}`},
		{name: "emoji 4000", body: fmt.Sprintf(`{"prompt":"%s","aspect":"landscape"}`, strings.Repeat("😀", 4000)), accepted: true},
		{name: "emoji 4001", body: fmt.Sprintf(`{"prompt":"%s","aspect":"landscape"}`, strings.Repeat("😀", 4001))},
		{name: "Chinese 4000", body: fmt.Sprintf(`{"prompt":"%s","aspect":"landscape"}`, strings.Repeat("山", 4000)), accepted: true},
		{name: "combining 4000", body: fmt.Sprintf(`{"prompt":"%s","aspect":"landscape"}`, strings.Repeat("e\u0301", 2000)), accepted: true},
	}
	for _, field := range []string{
		"model", "size", "n", "quality", "output_format", "response_format",
		"background", "stream", "group", "url", "file",
	} {
		testCases = append(testCases, struct {
			name     string
			body     string
			accepted bool
		}{name: "rejects client " + field, body: fmt.Sprintf(`{"prompt":"safe","aspect":"landscape","%s":"override"}`, field)})
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/pg/images/generations", strings.NewReader(testCase.body))
			c.Request.Header.Set("Content-Type", gin.MIMEJSON)

			err := normalizeImageStudioRequest(c)
			if !testCase.accepted {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			storage, storageErr := common.GetBodyStorage(c)
			require.NoError(t, storageErr)
			body, bodyErr := storage.Bytes()
			require.NoError(t, bodyErr)
			require.Contains(t, string(body), `"model":"gpt-image-2"`)
		})
	}
}

func TestImageStudioNormalizationDoesNotChangePublicImageOrPlaygroundChatRequests(t *testing.T) {
	testCases := []struct {
		name          string
		path          string
		body          string
		expectedModel string
	}{
		{name: "public image", path: "/v1/images/generations", body: `{"model":"dall-e-3","prompt":"public"}`, expectedModel: "dall-e-3"},
		{name: "playground chat", path: "/pg/chat/completions", body: `{"model":"chat-model","group":"default"}`, expectedModel: "chat-model"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", testCase.path, strings.NewReader(testCase.body))
			c.Request.Header.Set("Content-Type", gin.MIMEJSON)

			modelRequest, _, err := getModelRequest(c)
			require.NoError(t, err)
			require.Equal(t, testCase.expectedModel, modelRequest.Model)
			storage, storageErr := common.GetBodyStorage(c)
			require.NoError(t, storageErr)
			body, bodyErr := storage.Bytes()
			require.NoError(t, bodyErr)
			require.JSONEq(t, testCase.body, string(body))
		})
	}
}
