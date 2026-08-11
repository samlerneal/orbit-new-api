package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageEditChannelTestSendsOneSyntheticMultipartRequest(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratioMap := ratio_setting.GetModelRatioCopy()
	ratioMap["gpt-image-2"] = 1
	ratioData, err := common.Marshal(ratioMap)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(2<<20))
		require.Equal(t, "gpt-image-2", r.FormValue("model"))
		require.Equal(t, "Apply a subtle neutral edit.", r.FormValue("prompt"))
		require.Equal(t, "1", r.FormValue("n"))
		require.Equal(t, "1024x1024", r.FormValue("size"))
		require.Equal(t, "low", r.FormValue("quality"))
		files := r.MultipartForm.File["image"]
		require.Len(t, files, 1)
		file, err := files[0].Open()
		require.NoError(t, err)
		defer file.Close()
		decoded, err := png.Decode(file)
		require.NoError(t, err)
		require.Equal(t, 512, decoded.Bounds().Dx())
		require.Equal(t, 512, decoded.Bounds().Dy())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"url":"https://example.test/image.png"}],"usage":{}}`)
	}))
	defer server.Close()
	insertModelListUser(t, db, 901, "image-edit-tester", "default")
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)
	result := testChannel(context.Background(), channel, 901, "gpt-image-2", string(constant.EndpointTypeImageEdit), false)
	require.NoError(t, result.localErr)
	require.Equal(t, 1, requests)
}

func TestImageCapabilityEditSendsAllManifestFieldsAndFiveSyntheticReferences(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratios := ratio_setting.GetModelRatioCopy()
	ratios[imageCapabilityModel] = 1
	ratioData, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(8<<20))
		assert.Equal(t, imageCapabilityModel, r.FormValue("model"))
		assert.Equal(t, "4", r.FormValue("n"))
		assert.Equal(t, "4096x4096", r.FormValue("size"))
		assert.Equal(t, "high", r.FormValue("quality"))
		assert.Equal(t, "webp", r.FormValue("output_format"))
		assert.Equal(t, "transparent", r.FormValue("background"))
		files := r.MultipartForm.File["image[]"]
		require.Len(t, files, 5)
		for _, fileHeader := range files {
			file, openErr := fileHeader.Open()
			require.NoError(t, openErr)
			decoded, decodeErr := png.Decode(file)
			_ = file.Close()
			require.NoError(t, decodeErr)
			assert.Equal(t, 512, decoded.Bounds().Dx())
			assert.Equal(t, 512, decoded.Bounds().Dy())
		}
		_, _ = io.WriteString(w, `{"data":[{"url":"https://example.test/1"},{"url":"https://example.test/2"},{"url":"https://example.test/3"},{"url":"https://example.test/4"}]}`)
	}))
	defer server.Close()
	insertModelListUser(t, db, 904, "image-capability-edit", "default")
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image capability", Key: "test-key", BaseURL: &baseURL, Models: imageCapabilityModel, Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: imageCapabilityModel, ChannelId: channel.Id, Enabled: true}).Error)
	request := imageCapabilityRequest{SchemaVersion: imageCapabilitySchemaVersion, Model: imageCapabilityModel, Mode: "edit", Shape: "square", Resolution: "4K", N: 4, Quality: "high", Format: "webp", Background: "transparent", ReferenceCount: 5, Stream: false, ConfirmPaidImageProbe: true, ExpectedUpstreamRequests: 1}
	ctx := context.WithValue(context.Background(), imageCapabilityContextKey{}, request)
	result := testChannel(ctx, channel, 904, imageCapabilityModel, string(constant.EndpointTypeImageEdit), false)
	require.NoError(t, result.localErr)
	require.Equal(t, 1, requests)
}

func TestRunImageCapabilityGenerationUsesOneControlledRequestAndReturnsBase64Metadata(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	insertModelListUser(t, db, 905, "image-capability-generation", "default")
	imageBytes := bytes.Buffer{}
	require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 1024, 1024))))
	encoded := base64.StdEncoding.EncodeToString(imageBytes.Bytes())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, imageCapabilityModel, payload["model"])
		assert.NotEmpty(t, payload["prompt"])
		assert.Equal(t, float64(1), payload["n"])
		assert.Equal(t, "1024x1024", payload["size"])
		assert.Equal(t, "low", payload["quality"])
		assert.Equal(t, "png", payload["output_format"])
		assert.Equal(t, "opaque", payload["background"])
		w.Header().Set("X-Request-Id", "sensitive-request-id")
		w.Header().Set("X-Task-Id", "sensitive-task-id")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+encoded+`"}],"usage":{"input_tokens":1},"billable":true,"cost":5}`)
	}))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{Id: imageCapabilityCandidateID, Type: constant.ChannelTypeOpenAI, Name: "Codex", Key: "test-key", BaseURL: &baseURL, Models: imageCapabilityModel, Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: imageCapabilityModel, ChannelId: imageCapabilityCandidateID, Enabled: true}).Error)
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratios := ratio_setting.GetModelRatioCopy()
	ratios[imageCapabilityModel] = 1
	ratioData, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))
	request := imageCapabilityCases()[0].Request
	body, err := json.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Set("id", 905)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test/1/image-capability/run", bytes.NewReader(body))
	RunImageCapability(ctx)
	var response struct {
		Success   bool
		CaseID    string `json:"case_id"`
		State     string
		LatencyMS int64 `json:"latency_ms"`
		Data      map[string]any
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, request.CaseID, response.CaseID)
	assert.Equal(t, "OBSERVED_SUPPORTED", response.State)
	assert.Equal(t, float64(1), response.Data["actual_image_count"])
	assert.Equal(t, []any{"1024x1024"}, response.Data["dimensions"])
	assert.Equal(t, "png", response.Data["result_format"])
	assert.Equal(t, true, response.Data["request_id_present"])
	assert.Equal(t, true, response.Data["task_id_present"])
	assert.Equal(t, true, response.Data["usage_present"])
	assert.Equal(t, true, response.Data["billable_present"])
	assert.Equal(t, true, response.Data["cost_present"])
	assert.NotContains(t, recorder.Body.String(), encoded)
	assert.NotContains(t, recorder.Body.String(), "sensitive-request-id")
	assert.NotContains(t, recorder.Body.String(), "sensitive-task-id")
	assert.NotContains(t, recorder.Body.String(), "upstream_body_bytes")
	assert.Equal(t, 1, requests)
}

func TestImageCapabilityHandlerRejectsBeforeUpstream(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{Id: imageCapabilityCandidateID, Type: constant.ChannelTypeOpenAI, Name: "Codex", BaseURL: &baseURL, Models: imageCapabilityModel, Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	imageCapabilityInFlight.Store(false)
	defer imageCapabilityInFlight.Store(false)
	cases := imageCapabilityCases()
	for _, test := range []struct {
		name, id string
		request  imageCapabilityRequest
		want     string
	}{
		{name: "identity-drift", id: "2", request: imageCapabilityCases()[0].Request, want: "IMAGE_PROBE_CANDIDATE_IDENTITY_MISMATCH"},
		{name: "unsafe", id: "1", request: cases[len(cases)-1].Request, want: "IMAGE_PROBE_LOCAL_RESPONSE_LIMIT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Params = gin.Params{{Key: "id", Value: test.id}}
			body, err := json.Marshal(test.request)
			require.NoError(t, err)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			RunImageCapability(ctx)
			assert.Contains(t, recorder.Body.String(), test.want)
			assert.Equal(t, 0, requests)
		})
	}
	for _, payload := range [][]byte{[]byte(`{"case_id":"img-01"}`), []byte(`{"case_id":"img-01","unexpected":true}`)} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "id", Value: "1"}}
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
		RunImageCapability(ctx)
		assert.Contains(t, recorder.Body.String(), "IMAGE_PROBE_INVALID_JSON")
		assert.Equal(t, 0, requests)
	}
	imageCapabilityInFlight.Store(true)
	defer imageCapabilityInFlight.Store(false)
	body, err := json.Marshal(imageCapabilityCases()[0].Request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	RunImageCapability(ctx)
	assert.Contains(t, recorder.Body.String(), "IMAGE_PROBE_BUSY")
	assert.Equal(t, 0, requests)
}

func TestImageEditTestChannelPreflightRejectsBeforeUpstream(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	tests := []string{
		"endpoint_type=image-edit&model=gpt-image-2",
		"endpoint_type=image-edit&confirm_paid_image_edit=true",
		"endpoint_type=image-edit&model=gpt-image-2&confirm_paid_image_edit=true&stream=true",
	}
	for _, query := range tests {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/"+fmt.Sprintf("%d", channel.Id)+"?"+query, nil)
		TestChannel(ctx)
		var response struct {
			Success   bool            `json:"success"`
			ErrorCode types.ErrorCode `json:"error_code"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		assert.Equal(t, types.ErrorCodeInvalidRequest, response.ErrorCode)
		assert.Equal(t, 0, requests)
	}
}

func TestImageEditChannelTestRejectsOversizedErrorResponse(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratios := ratio_setting.GetModelRatioCopy()
	ratios["gpt-image-2"] = 1
	data, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(2<<20))
		require.Equal(t, "gpt-image-2", r.FormValue("model"))
		require.Equal(t, "Apply a subtle neutral edit.", r.FormValue("prompt"))
		file, err := r.MultipartForm.File["image"][0].Open()
		require.NoError(t, err)
		defer file.Close()
		decoded, err := png.Decode(file)
		require.NoError(t, err)
		require.Equal(t, 512, decoded.Bounds().Dx())
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "secret-error-body-"+strings.Repeat("x", (1<<20)+1))
	}))
	defer server.Close()
	insertModelListUser(t, db, 902, "image-edit-cap", "default")
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)
	result := testChannel(context.Background(), channel, 902, "gpt-image-2", string(constant.EndpointTypeImageEdit), false)
	require.Equal(t, 1, requests)
	require.EqualError(t, result.localErr, "image edit response exceeds the allowed size")
	require.NotNil(t, result.newAPIError)
	require.Equal(t, types.ErrorCodeBadResponseBody, result.newAPIError.GetErrorCode())
	for _, secret := range []string{"secret-error-body", "Apply a subtle neutral edit.", "test-key", "b64"} {
		require.NotContains(t, result.localErr.Error(), secret)
	}
}

func TestImageEditChannelTestRejectsOversizedSuccessResponse(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	original := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(original)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratios := ratio_setting.GetModelRatioCopy()
	ratios["gpt-image-2"] = 1
	data, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(2<<20))
		require.Equal(t, "gpt-image-2", r.FormValue("model"))
		file, err := r.MultipartForm.File["image"][0].Open()
		require.NoError(t, err)
		defer file.Close()
		decoded, err := png.Decode(file)
		require.NoError(t, err)
		require.Equal(t, 512, decoded.Bounds().Dx())
		_, _ = io.WriteString(w, "secret-success-body-"+strings.Repeat("x", (64<<20)+1))
	}))
	defer server.Close()
	insertModelListUser(t, db, 903, "image-edit-success-cap", "default")
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)
	result := testChannel(context.Background(), channel, 903, "gpt-image-2", string(constant.EndpointTypeImageEdit), false)
	require.Equal(t, 1, requests)
	require.EqualError(t, result.localErr, "image edit response exceeds the allowed size")
	require.NotNil(t, result.newAPIError)
	require.Equal(t, types.ErrorCodeBadResponseBody, result.newAPIError.GetErrorCode())
	for _, secret := range []string{"secret-success-body", "Apply a subtle neutral edit.", "test-key", "b64"} {
		require.NotContains(t, result.localErr.Error(), secret)
	}
}

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestImageEditChannelTestContract(t *testing.T) {
	endpoint, ok := common.GetDefaultEndpointInfo(constant.EndpointTypeImageEdit)
	require.True(t, ok)
	assert.Equal(t, "/v1/images/edits", endpoint.Path)

	request, ok := buildTestRequest("gpt-image-2", string(constant.EndpointTypeImageEdit), nil, false).(*dto.ImageRequest)
	require.True(t, ok)
	assert.Equal(t, "gpt-image-2", request.Model)
	assert.Equal(t, "Apply a subtle neutral edit.", request.Prompt)
	require.NotNil(t, request.N)
	assert.Equal(t, uint(1), *request.N)
	assert.Equal(t, "1024x1024", request.Size)
	assert.Equal(t, "low", request.Quality)
}

func TestImageEditResponseValidation(t *testing.T) {
	assert.True(t, isValidImageEditTestResponse([]byte(`{"data":[{"url":"https://example.test/image.png"}]}`)))
	assert.True(t, isValidImageEditTestResponse([]byte(`{"data":[{"b64_json":"encoded"}]}`)))
	assert.False(t, isValidImageEditTestResponse([]byte(`{"data":[]}`)))
	assert.False(t, isValidImageEditTestResponse([]byte(`{"data":[{}]}`)))
}

func TestImageCapabilityRequestRejectsUnsafeOrIncompletePaidProbe(t *testing.T) {
	request := imageCapabilityRequest{
		CaseID: "img-02", SchemaVersion: imageCapabilitySchemaVersion, Model: imageCapabilityModel,
		Mode: "edit", Shape: "square", Resolution: "1K", N: 1, Quality: "low",
		Format: "png", Background: "opaque", ReferenceCount: 1,
		Stream: false, ConfirmPaidImageProbe: true, ExpectedUpstreamRequests: 1,
	}
	require.NoError(t, validateImageCapabilityRequest(request))
	for _, mutate := range []func(*imageCapabilityRequest){
		func(r *imageCapabilityRequest) { r.Model = "another-model" },
		func(r *imageCapabilityRequest) { r.ReferenceCount = 0 },
		func(r *imageCapabilityRequest) { r.Stream = true },
		func(r *imageCapabilityRequest) { r.ConfirmPaidImageProbe = false },
		func(r *imageCapabilityRequest) { r.ExpectedUpstreamRequests = 2 },
	} {
		invalid := request
		mutate(&invalid)
		require.Error(t, validateImageCapabilityRequest(invalid))
	}
}

func TestImageCapabilityManifestIsDeterministicAndBounded(t *testing.T) {
	first := imageCapabilityManifest()
	second := imageCapabilityManifest()
	assert.Equal(t, first, second)
	firstBytes, firstErr := json.Marshal(first["cases"])
	secondBytes, secondErr := json.Marshal(second["cases"])
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, firstBytes, secondBytes)
	assert.Equal(t, "sha256:0ff09990f952553227569b5a80bf553ddd569efe62ff0e90db348d197f7f600b", first["hash"])
	assert.Equal(t, 44, first["unpruned_request_upper_bound"])
	assert.Equal(t, 42, first["executable_request_upper_bound"])
	assert.Equal(t, 82, first["unpruned_image_upper_bound"])
	assert.Equal(t, 74, first["executable_image_upper_bound"])
	cases, ok := first["cases"].([]imageCapabilityCase)
	require.True(t, ok)
	require.Len(t, cases, 44)
	stageCounts := map[string]int{}
	uniqueTuples := map[string]bool{}
	for _, candidate := range cases {
		stageCounts[candidate.Stage]++
		assert.Equal(t, fmt.Sprintf("img-%02d", len(uniqueTuples)+1), candidate.ID)
		request := candidate.Request
		tuple := fmt.Sprintf("%s/%s/%s/%d/%s/%s/%s/%d", request.Mode, request.Shape, request.Resolution, request.N, request.Quality, request.Format, request.Background, request.ReferenceCount)
		assert.False(t, uniqueTuples[tuple], "duplicate tuple %s", tuple)
		uniqueTuples[tuple] = true
	}
	assert.Equal(t, map[string]int{"baseline": 2, "axis": 32, "pairwise": 8, "worst-boundary": 2}, stageCounts)
	assert.Equal(t, "LOCALLY_UNSAFE_TO_PROBE", cases[42].State)
	assert.Equal(t, "LOCALLY_UNSAFE_TO_PROBE", cases[43].State)
	assert.Contains(t, fmt.Sprint(first["hash"]), "sha256:")
}

func TestImageCapabilityManifestPreservesIndependentHighValueAxes(t *testing.T) {
	seen4K, seenN4, seenPNG, seenHigh := false, false, false, false
	for _, candidate := range imageCapabilityCases() {
		if candidate.State == "LOCALLY_UNSAFE_TO_PROBE" {
			continue
		}
		seen4K = seen4K || candidate.Request.Resolution == "4K"
		seenN4 = seenN4 || candidate.Request.N == 4
		seenPNG = seenPNG || candidate.Request.Format == "png"
		seenHigh = seenHigh || candidate.Request.Quality == "high"
	}
	assert.True(t, seen4K && seenN4 && seenPNG && seenHigh)
}

func TestImageProbeResultMetadataDoesNotLeakImageBytesOrFetchURLs(t *testing.T) {
	imageBytes := bytes.Buffer{}
	require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 3))))
	encoded := base64.StdEncoding.EncodeToString(imageBytes.Bytes())
	meta := parseImageProbeResult([]byte(`{"data":[{"b64_json":"` + encoded + `"},{"url":"https://example.test/private.png"}]}`))
	assert.Equal(t, 2, meta.ActualImageCount)
	assert.Equal(t, []string{"UNKNOWN"}, meta.Dimensions)
	assert.Equal(t, "UNKNOWN", meta.Format)
	assert.NotContains(t, fmt.Sprint(meta), encoded)
}

func TestImageProbeResultKeepsURLOnlyMetadataUnknown(t *testing.T) {
	meta := parseImageProbeResult([]byte(`{"data":[{"url":"https://example.test/private.png"}]}`))
	assert.Equal(t, 1, meta.ActualImageCount)
	assert.Equal(t, []string{"UNKNOWN"}, meta.Dimensions)
	assert.Equal(t, "UNKNOWN", meta.Format)
}

func TestImageProbeExactCaseRequiresMatchingInlineDimensionsAndFormat(t *testing.T) {
	imageBytes := bytes.Buffer{}
	require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 3))))
	encoded := base64.StdEncoding.EncodeToString(imageBytes.Bytes())
	meta := parseImageProbeResult([]byte(`{"data":[{"b64_json":"` + encoded + `"}]}`))
	request := imageCapabilityCases()[0].Request
	request.Resolution = "4K"
	request.Format = "webp"
	assert.False(t, imageProbeMatchesExactCase(meta, request))
	request.Resolution = "1K"
	request.Format = "png"
	assert.False(t, imageProbeMatchesExactCase(meta, request))
	urlMeta := parseImageProbeResult([]byte(`{"data":[{"url":"https://example.test/image"}]}`))
	assert.False(t, imageProbeMatchesExactCase(urlMeta, imageCapabilityCases()[0].Request))
}

func TestImageProbeResultDetectsInlineWebPDimensions(t *testing.T) {
	webp := make([]byte, 30)
	copy(webp, "RIFF")
	copy(webp[8:], "WEBPVP8X")
	webp[24], webp[25], webp[26] = 1, 0, 0 // width = 2
	webp[27], webp[28], webp[29] = 2, 0, 0 // height = 3
	encoded := base64.StdEncoding.EncodeToString(webp)
	meta := parseImageProbeResult([]byte(`{"data":[{"b64_json":"` + encoded + `"}]}`))
	assert.Equal(t, 1, meta.ActualImageCount)
	assert.Equal(t, []string{"2x3"}, meta.Dimensions)
	assert.Equal(t, "webp", meta.Format)
}

func TestImageProbePresenceReturnsBooleansWithoutValues(t *testing.T) {
	presence := imageProbePresence(http.Header{"X-Request-Id": []string{"secret-id"}}, []byte(`{"task_id":"secret-task-id","usage":{"x":1},"cost":7}`))
	assert.Equal(t, true, presence["request_id_present"])
	assert.Equal(t, true, presence["task_id_present"])
	assert.Equal(t, true, presence["usage_present"])
	assert.Equal(t, false, presence["billable_present"])
	assert.Equal(t, true, presence["cost_present"])
	assert.NotContains(t, fmt.Sprint(presence), "secret-id")
	assert.NotContains(t, fmt.Sprint(presence), "secret-task-id")
	withoutIDs := imageProbePresence(http.Header{}, []byte(`{"usage":{}}`))
	assert.Equal(t, false, withoutIDs["request_id_present"])
	assert.Equal(t, false, withoutIDs["task_id_present"])
}

func TestImageCapabilityFailureResponseIsCaseScopedAndSanitized(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	request := imageCapabilityCases()[0].Request
	respondImageCapability(ctx, request, false, "UNVERIFIED", "upstream body: secret", 7, nil)
	assert.JSONEq(t, `{"success":false,"case_id":"img-01","state":"UNVERIFIED","latency_ms":7,"error_code":"IMAGE_PROBE_INTERNAL_ERROR"}`, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "secret")
}

func TestImageProbeFailureCodesAreSanitizedAndClassifiedWithoutErrorText(t *testing.T) {
	for statusCode, want := range map[int]string{
		http.StatusUnauthorized: "IMAGE_PROBE_UPSTREAM_AUTH_REJECTED",
	} {
		assert.Equal(t, want, imageProbeStatusErrorCode(statusCode))
	}
	assert.Equal(t, "IMAGE_PROBE_UPSTREAM_TIMEOUT", imageProbeTransportErrorCode(context.Background(), context.DeadlineExceeded))
	assert.Equal(t, "IMAGE_PROBE_UPSTREAM_TRANSPORT_FAILED", imageProbeTransportErrorCode(context.Background(), errors.New("transport secret")))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	respondImageCapability(ctx, imageCapabilityCases()[0].Request, false, "UNVERIFIED", "IMAGE_PROBE_RESPONSE_READ_FAILED", 0, nil)
	assert.Contains(t, recorder.Body.String(), "IMAGE_PROBE_RESPONSE_READ_FAILED")
	assert.NotContains(t, recorder.Body.String(), "secret")
	assert.Equal(t, "IMAGE_PROBE_INTERNAL_ERROR", imageProbeFailureResponseCode(testResult{localErr: errors.New("/internal/path stack secret")}))
	assert.Equal(t, "IMAGE_PROBE_INTERNAL_ERROR", imageProbeFailureResponseCode(testResult{localErr: errors.New("secret"), imageProbeErrorCode: "unknown"}))
	request := imageCapabilityCases()[0].Request
	for _, code := range []string{"IMAGE_PROBE_REQUEST_BUILD_FAILED", "IMAGE_PROBE_UPSTREAM_TIMEOUT", "IMAGE_PROBE_UPSTREAM_TRANSPORT_FAILED"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		respondImageCapability(ctx, request, false, "UNVERIFIED", imageProbeFailureResponseCode(testResult{localErr: errors.New("sensitive internal failure"), imageProbeErrorCode: code}), 0, nil)
		assert.JSONEq(t, `{"success":false,"case_id":"img-01","state":"UNVERIFIED","latency_ms":0,"error_code":"`+code+`"}`, recorder.Body.String())
		assert.NotContains(t, recorder.Body.String(), "sensitive")
	}
}

func TestRunImageCapabilityProjectsLocalUpstreamStatusWithoutSensitiveValues(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	service.InitHttpClient()
	const testUserID = 906
	insertModelListUser(t, db, testUserID, "image-probe-status", "default")
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	statusCode := http.StatusOK
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("X-Request-Id", "sensitive-request-id")
		w.Header().Set("Retry-After", "sensitive-retry-after")
		w.WriteHeader(statusCode)
		_, _ = io.WriteString(w, "sensitive-upstream-body")
	}))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{Id: imageCapabilityCandidateID, Type: constant.ChannelTypeOpenAI, Name: "Codex", Key: "sensitive-test-key", BaseURL: &baseURL, Models: imageCapabilityModel, Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: imageCapabilityModel, ChannelId: channel.Id, Enabled: true}).Error)
	ratioMap := ratio_setting.GetModelRatioCopy()
	ratioMap[imageCapabilityModel] = 1
	ratioData, err := common.Marshal(ratioMap)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))
	body, err := json.Marshal(imageCapabilityCases()[0].Request)
	require.NoError(t, err)
	for code, want := range map[int]string{
		http.StatusUnauthorized:    "IMAGE_PROBE_UPSTREAM_AUTH_REJECTED",
		http.StatusForbidden:       "IMAGE_PROBE_UPSTREAM_AUTH_REJECTED",
		http.StatusTooManyRequests: "IMAGE_PROBE_UPSTREAM_RATE_LIMITED",
		http.StatusBadGateway:      "IMAGE_PROBE_UPSTREAM_STATUS_REJECTED",
	} {
		t.Run(want, func(t *testing.T) {
			statusCode = code
			requests = 0
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Params = gin.Params{{Key: "id", Value: "1"}}
			ctx.Set("id", testUserID)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			RunImageCapability(ctx)
			assert.Equal(t, 1, requests)
			assertImageProbeFailureJSON(t, recorder.Body.String(), want)
		})
	}
}

func assertImageProbeFailureJSON(t *testing.T, body string, wantCode string) {
	t.Helper()
	var response map[string]json.RawMessage
	require.NoError(t, common.Unmarshal([]byte(body), &response))
	assert.Equal(t, map[string]struct{}{
		"success": {}, "case_id": {}, "state": {}, "latency_ms": {}, "error_code": {},
	}, func() map[string]struct{} {
		keys := make(map[string]struct{}, len(response))
		for key := range response {
			keys[key] = struct{}{}
		}
		return keys
	}())
	assert.JSONEq(t, "false", string(response["success"]))
	assert.JSONEq(t, `"img-01"`, string(response["case_id"]))
	assert.JSONEq(t, `"UNVERIFIED"`, string(response["state"]))
	assert.JSONEq(t, `"`+wantCode+`"`, string(response["error_code"]))
	for _, sensitive := range []string{"prompt", "b64", "task", "request-id", "secret-key", "/internal/", "stack", "sensitive"} {
		assert.NotContains(t, body, sensitive)
	}
}

func TestImageProbeTestChannelClassifiesBuildTimeoutAndTransportFailures(t *testing.T) {
	for _, test := range []struct {
		name            string
		timeout         bool
		useTransportURL bool
		configureRatios bool
		wantCode        string
		wantRequests    int
	}{
		{name: "build", wantCode: "IMAGE_PROBE_REQUEST_BUILD_FAILED", wantRequests: 0},
		{name: "timeout", timeout: true, configureRatios: true, wantCode: "IMAGE_PROBE_UPSTREAM_TIMEOUT", wantRequests: 0},
		{name: "transport", useTransportURL: true, configureRatios: true, wantCode: "IMAGE_PROBE_UPSTREAM_TRANSPORT_FAILED", wantRequests: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			service.InitHttpClient()
			insertModelListUser(t, db, 920, "image-probe-"+test.name, "default")
			originalRatios := ratio_setting.GetModelRatioCopy()
			t.Cleanup(func() {
				data, err := common.Marshal(originalRatios)
				require.NoError(t, err)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
			})
			if test.configureRatios {
				ratios := ratio_setting.GetModelRatioCopy()
				ratios[imageCapabilityModel] = 1
				data, err := common.Marshal(ratios)
				require.NoError(t, err)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				_, _ = io.WriteString(w, `{"data":[]}`)
			}))
			defer server.Close()
			baseURL := server.URL
			if test.useTransportURL {
				baseURL = "http://127.0.0.1:1"
			}
			channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image probe " + test.name, Key: "secret-key", BaseURL: &baseURL, Models: imageCapabilityModel, Group: "default", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: imageCapabilityModel, ChannelId: channel.Id, Enabled: true}).Error)
			request := imageCapabilityCases()[0].Request
			testContext := context.Background()
			if test.timeout {
				var cancel context.CancelFunc
				testContext, cancel = context.WithTimeout(testContext, 0)
				defer cancel()
			}
			result := testChannel(context.WithValue(testContext, imageCapabilityContextKey{}, request), channel, 920, imageCapabilityModel, string(constant.EndpointTypeImageGeneration), false)
			require.Error(t, result.localErr)
			assert.Equal(t, test.wantCode, imageProbeFailureResponseCode(result))
			assert.Equal(t, test.wantRequests, requests)
		})
	}
}

func TestRunImageCapabilityProjectsResponseFailuresWithoutSensitiveValues(t *testing.T) {
	for _, test := range []struct {
		name     string
		write    func(http.ResponseWriter)
		wantCode string
	}{
		{
			name: "response-limit",
			write: func(w http.ResponseWriter) {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", (64<<20)+1))
				_, _ = io.WriteString(w, "sensitive-large-body")
			},
			wantCode: "IMAGE_PROBE_RESPONSE_LIMIT_EXCEEDED",
		},
		{
			name: "invalid-response",
			write: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, "sensitive-invalid-json")
			},
			wantCode: "IMAGE_PROBE_RESPONSE_INVALID",
		},
		{
			name: "missing-result",
			write: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":[],"usage":{}}`)
			},
			wantCode: "IMAGE_PROBE_RESULT_MISSING",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			service.InitHttpClient()
			insertModelListUser(t, db, 921, "image-probe-response-"+test.name, "default")
			originalRatios := ratio_setting.GetModelRatioCopy()
			t.Cleanup(func() {
				data, err := common.Marshal(originalRatios)
				require.NoError(t, err)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
			})
			ratios := ratio_setting.GetModelRatioCopy()
			ratios[imageCapabilityModel] = 1
			ratioData, err := common.Marshal(ratios)
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, "/v1/images/generations", r.URL.Path)
				test.write(w)
			}))
			defer server.Close()
			baseURL := server.URL
			channel := &model.Channel{Id: imageCapabilityCandidateID, Type: constant.ChannelTypeOpenAI, Name: "Codex", Key: "secret-key", BaseURL: &baseURL, Models: imageCapabilityModel, Group: "default", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: imageCapabilityModel, ChannelId: channel.Id, Enabled: true}).Error)
			body, err := common.Marshal(imageCapabilityCases()[0].Request)
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Params = gin.Params{{Key: "id", Value: "1"}}
			ctx.Set("id", 921)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			RunImageCapability(ctx)
			assert.Equal(t, 1, requests)
			assertImageProbeFailureJSON(t, recorder.Body.String(), test.wantCode)
		})
	}
}

func TestImageCapabilityStrictJSONRejectsUnknownAndMissingFields(t *testing.T) {
	valid := imageCapabilityCases()[0].Request
	body, err := json.Marshal(valid)
	require.NoError(t, err)
	for _, payload := range [][]byte{append(append([]byte{}, body[:len(body)-1]...), []byte(`,"unexpected":true}`)...), []byte(`{"case_id":"img-01"}`)} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
		_, decodeErr := decodeImageCapabilityRequest(ctx)
		require.Error(t, decodeErr)
	}
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}
