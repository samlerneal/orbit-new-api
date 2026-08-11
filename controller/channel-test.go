package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/samber/lo"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

type testResult struct {
	context             *gin.Context
	localErr            error
	imageProbeErrorCode string
	newAPIError         *types.NewAPIError
	responseBody        []byte
	upstreamBodyBytes   int
	responseHeaders     http.Header
}

func imageProbeErrorCodeFor(isImageProbe bool, code string) string {
	if !isImageProbe {
		return ""
	}
	return code
}

func imageProbeTransportErrorCode(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "IMAGE_PROBE_UPSTREAM_TIMEOUT"
	}
	return "IMAGE_PROBE_UPSTREAM_TRANSPORT_FAILED"
}

func imageProbeStatusErrorCode(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "IMAGE_PROBE_UPSTREAM_AUTH_REJECTED"
	case http.StatusTooManyRequests:
		return "IMAGE_PROBE_UPSTREAM_RATE_LIMITED"
	default:
		return "IMAGE_PROBE_UPSTREAM_STATUS_REJECTED"
	}
}

const (
	imageCapabilitySchemaVersion = "image-channel-test.v1"
	imageCapabilityCandidateID   = 1
	imageCapabilityModel         = "gpt-image-2"
	imageCapabilityBodyLimit     = 16 << 10
)

var imageCapabilityInFlight atomic.Bool

// imageCapabilityRequest is intentionally complete. Keeping this contract on
// the server prevents a UI build from silently widening a paid probe.
type imageCapabilityRequest struct {
	CaseID                   string `json:"case_id"`
	SchemaVersion            string `json:"schema_version"`
	Model                    string `json:"model"`
	Mode                     string `json:"mode"`
	Shape                    string `json:"shape"`
	Resolution               string `json:"resolution"`
	N                        int    `json:"n"`
	Quality                  string `json:"quality"`
	Format                   string `json:"format"`
	Background               string `json:"background"`
	ReferenceCount           int    `json:"reference_count"`
	Stream                   bool   `json:"stream"`
	ConfirmPaidImageProbe    bool   `json:"confirm_paid_image_probe"`
	ExpectedUpstreamRequests int    `json:"expected_upstream_requests"`
}

type imageCapabilityContextKey struct{}

func imageCapabilitySize(request imageCapabilityRequest) string {
	base := map[string]map[string]string{
		"1K": {"square": "1024x1024", "landscape": "1536x1024", "portrait": "1024x1536"},
		"2K": {"square": "2048x2048", "landscape": "3072x2048", "portrait": "2048x3072"},
		"4K": {"square": "4096x4096", "landscape": "6144x4096", "portrait": "4096x6144"},
	}
	return base[request.Resolution][request.Shape]
}

func imageCapabilityRequestFromContext(ctx context.Context) (imageCapabilityRequest, bool) {
	request, ok := ctx.Value(imageCapabilityContextKey{}).(imageCapabilityRequest)
	return request, ok
}

type imageCapabilityMatrix struct {
	SchemaVersion string                 `json:"schema_version"`
	Candidate     map[string]interface{} `json:"candidate"`
	Axes          map[string]interface{} `json:"axes"`
	Manifest      map[string]interface{} `json:"manifest"`
}

type imageCapabilityCase struct {
	ID      string                 `json:"id"`
	Stage   string                 `json:"stage"`
	Request imageCapabilityRequest `json:"request"`
	State   string                 `json:"state"`
}

type imageProbeResultMeta struct {
	ActualImageCount  int      `json:"actual_image_count"`
	Dimensions        []string `json:"dimensions"`
	Format            string   `json:"result_format"`
	hasURL            bool
	hasInline         bool
	inlineParseFailed bool
}

func imageProbePresence(headers http.Header, body []byte) gin.H {
	return gin.H{
		"request_id_present": headers.Get("X-Request-Id") != "" || headers.Get("X-Request-ID") != "",
		"task_id_present":    headers.Get("X-Task-Id") != "" || headers.Get("X-Task-ID") != "" || gjson.GetBytes(body, "task_id").Exists(),
		"usage_present":      gjson.GetBytes(body, "usage").Exists(),
		"billable_present":   gjson.GetBytes(body, "billable").Exists(),
		"cost_present":       gjson.GetBytes(body, "cost").Exists(),
	}
}

func imageProbeSafeErrorCode(code string) string {
	if code == "IMAGE_PROBE_INVALID_MATRIX_REQUEST" {
		return "IMAGE_PROBE_CASE_MISMATCH"
	}
	switch code {
	case "", "IMAGE_PROBE_INVALID_JSON", "IMAGE_PROBE_CASE_MISMATCH", "IMAGE_PROBE_CASE_NOT_IN_MANIFEST", "IMAGE_PROBE_LOCAL_RESPONSE_LIMIT", "IMAGE_PROBE_BUSY", "IMAGE_PROBE_LOCAL_PRECONDITION_FAILED", "IMAGE_PROBE_UPSTREAM_UNVERIFIED", "IMAGE_PROBE_RESPONSE_METADATA_MISMATCH", "IMAGE_PROBE_CANDIDATE_IDENTITY_MISMATCH", "IMAGE_PROBE_REQUEST_BUILD_FAILED", "IMAGE_PROBE_UPSTREAM_TRANSPORT_FAILED", "IMAGE_PROBE_UPSTREAM_TIMEOUT", "IMAGE_PROBE_UPSTREAM_AUTH_REJECTED", "IMAGE_PROBE_UPSTREAM_RATE_LIMITED", "IMAGE_PROBE_UPSTREAM_STATUS_REJECTED", "IMAGE_PROBE_RESPONSE_LIMIT_EXCEEDED", "IMAGE_PROBE_RESPONSE_READ_FAILED", "IMAGE_PROBE_RESPONSE_INVALID", "IMAGE_PROBE_RESULT_MISSING", "IMAGE_PROBE_CLIENT_REQUEST_FAILED":
		return code
	default:
		return "IMAGE_PROBE_INTERNAL_ERROR"
	}
}

func imageProbeFailureResponseCode(result testResult) string {
	if result.localErr == nil {
		return ""
	}
	if result.imageProbeErrorCode == "" {
		return "IMAGE_PROBE_INTERNAL_ERROR"
	}
	return imageProbeSafeErrorCode(result.imageProbeErrorCode)
}

func respondImageCapability(c *gin.Context, request imageCapabilityRequest, success bool, state, errorCode string, latency int64, data gin.H) {
	response := gin.H{
		"success":    success,
		"case_id":    request.CaseID,
		"state":      state,
		"latency_ms": latency,
	}
	if code := imageProbeSafeErrorCode(errorCode); code != "" {
		response["error_code"] = code
	}
	if data != nil {
		response["data"] = data
	}
	c.JSON(http.StatusOK, response)
}

func decodeImageProbeConfig(decoded []byte) (image.Config, string, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err == nil {
		return config, format, nil
	}
	// WebP is an allowed output format but the Go standard image registry does
	// not decode it. VP8X carries the canvas dimensions without decoding pixels.
	if len(decoded) >= 30 && string(decoded[:4]) == "RIFF" && string(decoded[8:12]) == "WEBP" && string(decoded[12:16]) == "VP8X" {
		width := 1 + int(decoded[24]) + int(decoded[25])<<8 + int(decoded[26])<<16
		height := 1 + int(decoded[27]) + int(decoded[28])<<8 + int(decoded[29])<<16
		if width > 0 && height > 0 {
			return image.Config{Width: width, Height: height}, "webp", nil
		}
	}
	return image.Config{}, "", err
}

// parseImageProbeResult inspects inline image data only in memory. URLs are
// never fetched, and the returned metadata deliberately excludes all bytes.
func parseImageProbeResult(responseBody []byte) imageProbeResultMeta {
	meta := imageProbeResultMeta{Dimensions: []string{}, Format: "UNKNOWN"}
	data := gjson.GetBytes(responseBody, "data")
	if !data.IsArray() {
		return meta
	}
	seen := map[string]bool{}
	for _, entry := range data.Array() {
		if entry.Get("url").String() != "" {
			meta.ActualImageCount++
			meta.hasURL = true
			continue
		}
		encoded := entry.Get("b64_json").String()
		if encoded == "" {
			meta.ActualImageCount++
			meta.inlineParseFailed = true
			continue
		}
		meta.ActualImageCount++
		meta.hasInline = true
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			meta.inlineParseFailed = true
			continue
		}
		config, format, err := decodeImageProbeConfig(decoded)
		if err != nil {
			meta.inlineParseFailed = true
			continue
		}
		if meta.Format != "UNKNOWN" && meta.Format != format {
			meta.inlineParseFailed = true
		}
		meta.Format = format
		dimension := fmt.Sprintf("%dx%d", config.Width, config.Height)
		if !seen[dimension] {
			seen[dimension] = true
			meta.Dimensions = append(meta.Dimensions, dimension)
		}
	}
	if len(meta.Dimensions) == 0 {
		meta.Dimensions = []string{"UNKNOWN"}
	}
	if meta.hasURL {
		meta.Dimensions = []string{"UNKNOWN"}
		meta.Format = "UNKNOWN"
	}
	return meta
}

func imageProbeMatchesExactCase(meta imageProbeResultMeta, request imageCapabilityRequest) bool {
	return meta.hasInline && !meta.hasURL && !meta.inlineParseFailed &&
		meta.ActualImageCount == request.N && len(meta.Dimensions) == 1 &&
		meta.Dimensions[0] == imageCapabilitySize(request) && meta.Format == request.Format
}

func imageCapabilityCases() []imageCapabilityCase {
	base := func(mode string) imageCapabilityRequest {
		return imageCapabilityRequest{SchemaVersion: imageCapabilitySchemaVersion, Model: imageCapabilityModel, Mode: mode, Shape: "square", Resolution: "1K", N: 1, Quality: "low", Format: "png", Background: "opaque", ReferenceCount: map[string]int{"generation": 0, "edit": 1}[mode], Stream: false, ConfirmPaidImageProbe: true, ExpectedUpstreamRequests: 1}
	}
	cases := make([]imageCapabilityCase, 0, 52)
	add := func(stage string, request imageCapabilityRequest, state string) {
		request.CaseID = fmt.Sprintf("img-%02d", len(cases)+1)
		cases = append(cases, imageCapabilityCase{ID: request.CaseID, Stage: stage, Request: request, State: state})
	}
	for _, mode := range []string{"generation", "edit"} {
		add("baseline", base(mode), "NOT_PROBED")
	}
	// Each candidate value is represented by a safe one-axis case for both modes.
	for _, mode := range []string{"generation", "edit"} {
		for _, shape := range []string{"landscape", "portrait"} {
			r := base(mode)
			r.Shape = shape
			add("axis", r, "NOT_PROBED")
		}
		for _, resolution := range []string{"2K", "4K"} {
			r := base(mode)
			r.Resolution = resolution
			add("axis", r, "NOT_PROBED")
		}
		for _, n := range []int{2, 4} {
			r := base(mode)
			r.N = n
			add("axis", r, "NOT_PROBED")
		}
		for _, quality := range []string{"auto", "medium", "high"} {
			r := base(mode)
			r.Quality = quality
			add("axis", r, "NOT_PROBED")
		}
		for _, format := range []string{"jpeg", "webp"} {
			r := base(mode)
			r.Format = format
			add("axis", r, "NOT_PROBED")
		}
		for _, background := range []string{"auto", "transparent"} {
			r := base(mode)
			r.Background = background
			add("axis", r, "NOT_PROBED")
		}
	}
	for _, references := range []int{2, 5} {
		r := base("edit")
		r.ReferenceCount = references
		add("axis", r, "NOT_PROBED")
	}
	for len(cases) < 36 {
		r := base([]string{"generation", "edit"}[len(cases)%2])
		r.Shape = []string{"square", "landscape", "portrait"}[len(cases)%3]
		r.N = []int{1, 2, 4}[len(cases)%3]
		add("axis", r, "NOT_PROBED")
	}
	for len(cases) < 50 {
		r := base([]string{"generation", "edit"}[len(cases)%2])
		r.Shape = []string{"square", "landscape", "portrait"}[len(cases)%3]
		r.Resolution = []string{"1K", "2K", "4K"}[len(cases)%3]
		r.N = []int{1, 2, 4}[len(cases)%3]
		r.Quality = []string{"auto", "low", "medium", "high"}[len(cases)%4]
		r.Format = []string{"png", "jpeg", "webp"}[len(cases)%3]
		add("pairwise", r, "NOT_PROBED")
	}
	r := base("generation")
	r.Resolution = "4K"
	r.N = 4
	r.Quality = "high"
	r.Format = "png"
	add("worst-boundary", r, "LOCALLY_UNSAFE_TO_PROBE")
	r = base("edit")
	r.Resolution = "4K"
	r.N = 4
	r.Quality = "high"
	r.Format = "png"
	r.ReferenceCount = 5
	add("worst-boundary", r, "LOCALLY_UNSAFE_TO_PROBE")
	unique := make([]imageCapabilityCase, 0, len(cases))
	seen := make(map[string]struct{}, len(cases))
	for _, candidate := range cases {
		request := candidate.Request
		key := fmt.Sprintf("%s/%s/%s/%d/%s/%s/%s/%d", request.Mode, request.Shape, request.Resolution, request.N, request.Quality, request.Format, request.Background, request.ReferenceCount)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		request.CaseID = fmt.Sprintf("img-%02d", len(unique)+1)
		candidate.ID = request.CaseID
		candidate.Request = request
		unique = append(unique, candidate)
	}
	return unique
}

func imageCapabilityManifest() map[string]interface{} {
	cases := imageCapabilityCases()
	canonical, _ := json.Marshal(cases)
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	maximumImages, executableImages, executableRequests := 0, 0, 0
	for _, candidate := range cases {
		maximumImages += candidate.Request.N
		if candidate.State != "LOCALLY_UNSAFE_TO_PROBE" {
			executableRequests++
			executableImages += candidate.Request.N
		}
	}
	stageCounts := map[string]int{}
	for _, candidate := range cases {
		stageCounts[candidate.Stage]++
	}
	return map[string]interface{}{"algorithm": "progressive-pruning.v1", "hash": hash, "cases": cases, "stages": []map[string]interface{}{{"name": "baseline", "count": stageCounts["baseline"]}, {"name": "axis", "count": stageCounts["axis"]}, {"name": "pairwise", "count": stageCounts["pairwise"]}, {"name": "worst-boundary", "count": stageCounts["worst-boundary"]}}, "unpruned_request_upper_bound": len(cases), "executable_request_upper_bound": executableRequests, "unpruned_image_upper_bound": maximumImages, "executable_image_upper_bound": executableImages}
}

func imageCapabilityValues() imageCapabilityMatrix {
	return imageCapabilityMatrix{
		SchemaVersion: imageCapabilitySchemaVersion,
		Candidate:     map[string]interface{}{"channel_id": imageCapabilityCandidateID, "name": "Codex", "model": imageCapabilityModel},
		Axes: map[string]interface{}{
			"mode": []string{"generation", "edit"}, "shape": []string{"square", "landscape", "portrait"},
			"resolution": []string{"1K", "2K", "4K"}, "n": []int{1, 2, 4},
			"quality": []string{"auto", "low", "medium", "high"}, "format": []string{"png", "jpeg", "webp"},
			"background": []string{"auto", "opaque", "transparent"}, "edit_reference_count": []int{1, 2, 5},
		},
		Manifest: imageCapabilityManifest(),
	}
}

func containsImageCapabilityValue(values []string, value string) bool {
	return lo.Contains(values, value)
}

func validateImageCapabilityRequest(request imageCapabilityRequest) error {
	if request.SchemaVersion != imageCapabilitySchemaVersion || request.Model != imageCapabilityModel ||
		!containsImageCapabilityValue([]string{"generation", "edit"}, request.Mode) ||
		!containsImageCapabilityValue([]string{"square", "landscape", "portrait"}, request.Shape) ||
		!containsImageCapabilityValue([]string{"1K", "2K", "4K"}, request.Resolution) ||
		!lo.Contains([]int{1, 2, 4}, request.N) ||
		!containsImageCapabilityValue([]string{"auto", "low", "medium", "high"}, request.Quality) ||
		!containsImageCapabilityValue([]string{"png", "jpeg", "webp"}, request.Format) ||
		!containsImageCapabilityValue([]string{"auto", "opaque", "transparent"}, request.Background) ||
		(request.Mode == "generation" && request.ReferenceCount != 0) ||
		(request.Mode == "edit" && !lo.Contains([]int{1, 2, 5}, request.ReferenceCount)) ||
		request.Stream || !request.ConfirmPaidImageProbe || request.ExpectedUpstreamRequests != 1 {
		return errors.New("IMAGE_PROBE_INVALID_MATRIX_REQUEST")
	}
	for _, candidate := range imageCapabilityCases() {
		candidateRequest := candidate.Request
		candidateRequest.ConfirmPaidImageProbe = request.ConfirmPaidImageProbe
		if candidateRequest == request {
			return nil
		}
	}
	return errors.New("IMAGE_PROBE_CASE_NOT_IN_MANIFEST")
}

func imageCapabilityCandidate(channel *model.Channel) bool {
	if channel == nil || channel.Id != imageCapabilityCandidateID || strings.TrimSpace(channel.Name) != "Codex" || channel.Type != constant.ChannelTypeOpenAI {
		return false
	}
	return lo.Contains(channel.GetModels(), imageCapabilityModel)
}

func loadImageCapabilityCandidate(c *gin.Context) (*model.Channel, bool) {
	if c.Param("id") != strconv.Itoa(imageCapabilityCandidateID) {
		return nil, false
	}
	channel, err := model.GetChannelById(imageCapabilityCandidateID, true)
	if err != nil || !imageCapabilityCandidate(channel) {
		return nil, false
	}
	return channel, true
}

func imageCapabilityIdentityMismatch(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": false, "error_code": "IMAGE_PROBE_CANDIDATE_IDENTITY_MISMATCH"})
}

// GetImageCapability is read-only and intentionally performs no upstream IO.
func GetImageCapability(c *gin.Context) {
	if _, ok := loadImageCapabilityCandidate(c); !ok {
		imageCapabilityIdentityMismatch(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": imageCapabilityValues()})
}

func decodeImageCapabilityRequest(c *gin.Context) (imageCapabilityRequest, error) {
	var request imageCapabilityRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, imageCapabilityBodyLimit)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return request, errors.New("IMAGE_PROBE_INVALID_JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, errors.New("IMAGE_PROBE_INVALID_JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return request, errors.New("IMAGE_PROBE_INVALID_JSON")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || len(raw) != 14 {
		return request, errors.New("IMAGE_PROBE_INVALID_JSON")
	}
	for _, field := range []string{"case_id", "schema_version", "model", "mode", "shape", "resolution", "n", "quality", "format", "background", "reference_count", "stream", "confirm_paid_image_probe", "expected_upstream_requests"} {
		if _, ok := raw[field]; !ok {
			return request, errors.New("IMAGE_PROBE_INVALID_JSON")
		}
	}
	return request, validateImageCapabilityRequest(request)
}

// RunImageCapability is the only paid-probe entrypoint. It accepts one
// explicit case and never advances the manifest or retries automatically.
func RunImageCapability(c *gin.Context) {
	channel, ok := loadImageCapabilityCandidate(c)
	if !ok {
		imageCapabilityIdentityMismatch(c)
		return
	}
	request, err := decodeImageCapabilityRequest(c)
	if err != nil {
		respondImageCapability(c, request, false, "UNVERIFIED", err.Error(), 0, nil)
		return
	}
	caseState := "NOT_PROBED"
	for _, candidate := range imageCapabilityCases() {
		if candidate.ID == request.CaseID {
			caseState = candidate.State
			break
		}
	}
	if caseState == "LOCALLY_UNSAFE_TO_PROBE" {
		respondImageCapability(c, request, false, "LOCALLY_UNSAFE_TO_PROBE", "IMAGE_PROBE_LOCAL_RESPONSE_LIMIT", 0, nil)
		return
	}
	if !imageCapabilityInFlight.CompareAndSwap(false, true) {
		respondImageCapability(c, request, false, "UNVERIFIED", "IMAGE_PROBE_BUSY", 0, nil)
		return
	}
	defer imageCapabilityInFlight.Store(false)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, imageCapabilityContextKey{}, request)
	endpoint := string(constant.EndpointTypeImageGeneration)
	if request.Mode == "edit" {
		endpoint = string(constant.EndpointTypeImageEdit)
	}
	started := time.Now()
	testUserID, userErr := resolveChannelTestUserID(c)
	if userErr != nil {
		respondImageCapability(c, request, false, "UNVERIFIED", "IMAGE_PROBE_LOCAL_PRECONDITION_FAILED", 0, nil)
		return
	}
	result := testChannel(ctx, channel, testUserID, imageCapabilityModel, endpoint, false)
	latency := time.Since(started).Milliseconds()
	if result.localErr != nil {
		respondImageCapability(c, request, false, "UNVERIFIED", imageProbeFailureResponseCode(result), latency, nil)
		return
	}
	resultMeta := parseImageProbeResult(result.responseBody)
	if !imageProbeMatchesExactCase(resultMeta, request) {
		respondImageCapability(c, request, false, "UNVERIFIED", "IMAGE_PROBE_RESPONSE_METADATA_MISMATCH", latency, nil)
		return
	}
	metadata := gin.H{"mode": request.Mode, "shape": request.Shape, "resolution": request.Resolution, "n": request.N, "quality": request.Quality, "format": request.Format, "background": request.Background, "reference_count": request.ReferenceCount, "actual_image_count": resultMeta.ActualImageCount, "dimensions": resultMeta.Dimensions, "result_format": resultMeta.Format}
	for key, value := range imageProbePresence(result.responseHeaders, result.responseBody) {
		metadata[key] = value
	}
	respondImageCapability(c, request, true, "OBSERVED_SUPPORTED", "", latency, metadata)
}

func normalizeChannelTestEndpoint(channel *model.Channel, modelName, endpointType string) string {
	normalized := strings.TrimSpace(endpointType)
	if normalized != "" {
		return normalized
	}
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return string(constant.EndpointTypeOpenAIResponseCompact)
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	return normalized
}

func resolveChannelTestUserID(c *gin.Context) (int, error) {
	if c != nil {
		if userID := c.GetInt("id"); userID > 0 {
			return userID, nil
		}
	}

	var rootUser model.User
	if err := model.DB.Select("id").Where("role = ?", common.RoleRootUser).First(&rootUser).Error; err != nil {
		return 0, fmt.Errorf("failed to resolve channel test user: %w", err)
	}
	if rootUser.Id == 0 {
		return 0, errors.New("failed to resolve channel test user")
	}
	return rootUser.Id, nil
}

func testChannel(ctx context.Context, channel *model.Channel, testUserID int, testModel string, endpointType string, isStream bool) testResult {
	if ctx == nil {
		ctx = context.Background()
	}
	tik := time.Now()
	var unsupportedTestChannelTypes = []int{
		constant.ChannelTypeMidjourney,
		constant.ChannelTypeMidjourneyPlus,
		constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling,
		constant.ChannelTypeJimeng,
		constant.ChannelTypeDoubaoVideo,
		constant.ChannelTypeVidu,
	}
	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testModel = strings.TrimSpace(testModel)
	if testModel == "" {
		if channel.TestModel != nil && *channel.TestModel != "" {
			testModel = strings.TrimSpace(*channel.TestModel)
		} else {
			models := channel.GetModels()
			if len(models) > 0 {
				testModel = strings.TrimSpace(models[0])
			}
			if testModel == "" {
				testModel = "gpt-4o-mini"
			}
		}
	}

	endpointType = normalizeChannelTestEndpoint(channel, testModel, endpointType)

	isImageEditTest := constant.EndpointType(endpointType) == constant.EndpointTypeImageEdit
	isImageProbe := constant.EndpointType(endpointType) == constant.EndpointTypeImageGeneration || isImageEditTest
	probeRequest, hasProbeRequest := imageCapabilityRequestFromContext(ctx)
	if isImageEditTest && (testModel == "" || isStream) {
		return testResult{localErr: errors.New("image edit test requires an explicit model and stream=false"), imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED")}
	}

	requestPath := "/v1/chat/completions"

	// 如果指定了端点类型，使用指定的端点类型
	if endpointType != "" {
		if endpointInfo, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpointType)); ok {
			requestPath = endpointInfo.Path
		}
	} else {
		// 如果没有指定端点类型，使用原有的自动检测逻辑

		if strings.Contains(strings.ToLower(testModel), "rerank") {
			requestPath = "/v1/rerank"
		}

		// 先判断是否为 Embedding 模型
		if strings.Contains(strings.ToLower(testModel), "embedding") ||
			strings.HasPrefix(testModel, "m3e") || // m3e 系列模型
			strings.Contains(testModel, "bge-") || // bge 系列模型
			strings.Contains(testModel, "embed") ||
			channel.Type == constant.ChannelTypeMokaAI { // 其他 embedding 模型
			requestPath = "/v1/embeddings" // 修改请求路径
		}

		// VolcEngine 图像生成模型
		if channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(testModel, "seedream") {
			requestPath = "/v1/images/generations"
		}

		// responses-only models
		if strings.Contains(strings.ToLower(testModel), "codex") {
			requestPath = "/v1/responses"
		}

		// responses compaction models (must use /v1/responses/compact)
		if strings.HasSuffix(testModel, ratio_setting.CompactModelSuffix) {
			requestPath = "/v1/responses/compact"
		}
	}
	if strings.HasPrefix(requestPath, "/v1/responses/compact") {
		testModel = ratio_setting.WithCompactModelSuffix(testModel)
	}

	if isImageEditTest {
		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)
		formValues := map[string]string{
			"model":   testModel,
			"prompt":  "Apply a subtle neutral edit.",
			"n":       "1",
			"size":    "1024x1024",
			"quality": "low",
		}
		if hasProbeRequest {
			formValues = map[string]string{
				"model": testModel, "prompt": "Apply a neutral transformation to the supplied solid-color image.",
				"n": strconv.Itoa(probeRequest.N), "size": imageCapabilitySize(probeRequest), "quality": probeRequest.Quality,
				"output_format": probeRequest.Format, "background": probeRequest.Background,
			}
		}
		for key, value := range formValues {
			if err := writer.WriteField(key, value); err != nil {
				return testResult{localErr: fmt.Errorf("build image edit test form: %w", err), imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED")}
			}
		}
		referenceCount := 1
		if hasProbeRequest {
			referenceCount = probeRequest.ReferenceCount
		}
		for referenceIndex := 0; referenceIndex < referenceCount; referenceIndex++ {
			imagePart, err := writer.CreateFormFile("image", fmt.Sprintf("test-image-%d.png", referenceIndex+1))
			if err != nil {
				return testResult{localErr: fmt.Errorf("build image edit test image: %w", err), imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED")}
			}
			pngImage := image.NewRGBA(image.Rect(0, 0, 512, 512))
			fill := uint8(96 + referenceIndex*24)
			for y := 0; y < 512; y++ {
				for x := 0; x < 512; x++ {
					pngImage.SetRGBA(x, y, color.RGBA{R: fill, G: 128, B: 160, A: 255})
				}
			}
			if err := png.Encode(imagePart, pngImage); err != nil {
				return testResult{localErr: fmt.Errorf("encode image edit test image: %w", err), imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED")}
			}
		}
		if err := writer.Close(); err != nil {
			return testResult{localErr: fmt.Errorf("finalize image edit test form: %w", err), imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED")}
		}
		c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, requestPath, &requestBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	} else {
		c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, requestPath, nil)
	}

	cache, err := model.GetUserCache(testUserID)
	if err != nil {
		return testResult{
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
			newAPIError:         nil,
		}
	}
	cache.WriteContext(c)
	c.Set("id", testUserID)

	//c.Request.Header.Set("Authorization", "Bearer "+channel.Key)
	if !isImageProbe {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(testUserID, false)
	c.Set("group", group)

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, testModel)
	if newAPIError != nil {
		return testResult{
			context:             c,
			localErr:            newAPIError,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
			newAPIError:         newAPIError,
		}
	}

	// Determine relay format based on endpoint type or request path
	var relayFormat types.RelayFormat
	if endpointType != "" {
		// 根据指定的端点类型设置 relayFormat
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeOpenAI:
			relayFormat = types.RelayFormatOpenAI
		case constant.EndpointTypeOpenAIResponse:
			relayFormat = types.RelayFormatOpenAIResponses
		case constant.EndpointTypeOpenAIResponseCompact:
			relayFormat = types.RelayFormatOpenAIResponsesCompaction
		case constant.EndpointTypeAnthropic:
			relayFormat = types.RelayFormatClaude
		case constant.EndpointTypeGemini:
			relayFormat = types.RelayFormatGemini
		case constant.EndpointTypeJinaRerank:
			relayFormat = types.RelayFormatRerank
		case constant.EndpointTypeImageGeneration, constant.EndpointTypeImageEdit:
			relayFormat = types.RelayFormatOpenAIImage
		case constant.EndpointTypeEmbeddings:
			relayFormat = types.RelayFormatEmbedding
		default:
			relayFormat = types.RelayFormatOpenAI
		}
	} else {
		// 根据请求路径自动检测
		relayFormat = types.RelayFormatOpenAI
		if c.Request.URL.Path == "/v1/embeddings" {
			relayFormat = types.RelayFormatEmbedding
		}
		if c.Request.URL.Path == "/v1/images/generations" {
			relayFormat = types.RelayFormatOpenAIImage
		}
		if c.Request.URL.Path == "/v1/messages" {
			relayFormat = types.RelayFormatClaude
		}
		if strings.Contains(c.Request.URL.Path, "/v1beta/models") {
			relayFormat = types.RelayFormatGemini
		}
		if c.Request.URL.Path == "/v1/rerank" || c.Request.URL.Path == "/rerank" {
			relayFormat = types.RelayFormatRerank
		}
		if c.Request.URL.Path == "/v1/responses" {
			relayFormat = types.RelayFormatOpenAIResponses
		}
		if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") {
			relayFormat = types.RelayFormatOpenAIResponsesCompaction
		}
	}

	var request dto.Request
	if isImageEditTest {
		request, err = helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		if err != nil {
			return testResult{context: c, localErr: err, imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"), newAPIError: types.NewError(err, types.ErrorCodeInvalidRequest)}
		}
	} else {
		request = buildTestRequest(testModel, endpointType, channel, isStream)
		if hasProbeRequest && isImageProbe {
			request = &dto.ImageRequest{Model: testModel, Prompt: "Create a neutral abstract solid-color composition.", N: lo.ToPtr(uint(probeRequest.N)), Size: imageCapabilitySize(probeRequest), Quality: probeRequest.Quality, OutputFormat: json.RawMessage(strconv.Quote(probeRequest.Format)), Background: json.RawMessage(strconv.Quote(probeRequest.Background))}
		}
	}

	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)

	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
			newAPIError:         types.NewError(err, types.ErrorCodeGenRelayInfoFailed),
		}
	}

	info.IsChannelTest = true
	info.InitChannelMeta(c)

	err = attachTestBillingRequestInput(info, request)
	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
			newAPIError:         types.NewError(err, types.ErrorCodeJsonMarshalFailed),
		}
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
			newAPIError:         types.NewError(err, types.ErrorCodeChannelModelMappedError),
		}
	}

	testModel = info.UpstreamModelName
	// 更新请求中的模型名称
	request.SetModelName(testModel)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		apiType != constant.APITypeOpenAI &&
		apiType != constant.APITypeCodex {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("responses compaction test only supports openai/codex channels, got api type %d", apiType),
			newAPIError: types.NewError(fmt.Errorf("unsupported api type: %d", apiType), types.ErrorCodeInvalidApiType),
		}
	}
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("invalid api type: %d, adaptor is nil", apiType),
			newAPIError: types.NewError(fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), types.ErrorCodeInvalidApiType),
		}
	}

	//// 创建一个用于日志的 info 副本，移除 ApiKey
	//logInfo := info
	//logInfo.ApiKey = ""
	if !isImageProbe {
		common.SysLog(fmt.Sprintf("testing channel %d with model %s , info %+v ", channel.Id, testModel, info.ToString()))
	}

	priceData, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
			newAPIError:         types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest)),
		}
	}

	adaptor.Init(info)

	var convertedRequest any
	// 根据 RelayMode 选择正确的转换函数
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		// Embedding 请求 - request 已经是正确的类型
		if embeddingReq, ok := request.(*dto.EmbeddingRequest); ok {
			convertedRequest, err = adaptor.ConvertEmbeddingRequest(c, info, *embeddingReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid embedding request type"),
				newAPIError: types.NewError(errors.New("invalid embedding request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeImagesGenerations:
		// 图像生成请求 - request 已经是正确的类型
		if imageReq, ok := request.(*dto.ImageRequest); ok {
			convertedRequest, err = adaptor.ConvertImageRequest(c, info, *imageReq)
		} else {
			return testResult{
				context:             c,
				localErr:            errors.New("invalid image request type"),
				imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
				newAPIError:         types.NewError(errors.New("invalid image request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeImagesEdits:
		if imageReq, ok := request.(*dto.ImageRequest); ok {
			convertedRequest, err = adaptor.ConvertImageRequest(c, info, *imageReq)
		} else {
			return testResult{
				context:             c,
				localErr:            errors.New("invalid image edit request type"),
				imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
				newAPIError:         types.NewError(errors.New("invalid image edit request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeRerank:
		// Rerank 请求 - request 已经是正确的类型
		if rerankReq, ok := request.(*dto.RerankRequest); ok {
			convertedRequest, err = adaptor.ConvertRerankRequest(c, info.RelayMode, *rerankReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid rerank request type"),
				newAPIError: types.NewError(errors.New("invalid rerank request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeResponses:
		// Response 请求 - request 已经是正确的类型
		if responseReq, ok := request.(*dto.OpenAIResponsesRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *responseReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid response request type"),
				newAPIError: types.NewError(errors.New("invalid response request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeResponsesCompact:
		// Response compaction request - convert to OpenAIResponsesRequest before adapting
		switch req := request.(type) {
		case *dto.OpenAIResponsesCompactionRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
				Model:              req.Model,
				Input:              req.Input,
				Instructions:       req.Instructions,
				PreviousResponseID: req.PreviousResponseID,
			})
		case *dto.OpenAIResponsesRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *req)
		default:
			return testResult{
				context:     c,
				localErr:    errors.New("invalid response compaction request type"),
				newAPIError: types.NewError(errors.New("invalid response compaction request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	default:
		// Chat/Completion 等其他请求类型
		if generalReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIRequest(c, info, generalReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid general request type"),
				newAPIError: types.NewError(errors.New("invalid general request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	}

	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
			newAPIError:         types.NewError(err, types.ErrorCodeConvertRequestFailed),
		}
	}
	var requestBody io.Reader
	var jsonData []byte
	if isImageEditTest {
		convertedBody, ok := convertedRequest.(*bytes.Buffer)
		if !ok {
			return testResult{context: c, localErr: errors.New("invalid image edit multipart conversion"), imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"), newAPIError: types.NewError(errors.New("invalid image edit multipart conversion"), types.ErrorCodeConvertRequestFailed)}
		}
		requestBody = convertedBody
	} else {
		jsonData, err = common.Marshal(convertedRequest)
		if err != nil {
			return testResult{
				context:             c,
				localErr:            err,
				imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
				newAPIError:         types.NewError(err, types.ErrorCodeJsonMarshalFailed),
			}
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	//jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
	//if err != nil {
	//	return testResult{
	//		context:     c,
	//		localErr:    err,
	//		newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
	//	}
	//}

	if len(info.ParamOverride) > 0 && !isImageEditTest {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
				return testResult{
					context:     c,
					localErr:    fixedErr,
					newAPIError: relaycommon.NewAPIErrorFromParamOverride(fixedErr),
				}
			}
			return testResult{
				context:             c,
				localErr:            err,
				imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_REQUEST_BUILD_FAILED"),
				newAPIError:         types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid),
			}
		}
	}

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, imageProbeTransportErrorCode(ctx, err)),
			newAPIError:         types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
		}
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if isImageProbe {
			maxResponseBytes := int64(64 << 20)
			if httpResp.StatusCode != http.StatusOK {
				maxResponseBytes = 1 << 20
			}
			if httpResp.ContentLength > maxResponseBytes {
				_ = httpResp.Body.Close()
				return testResult{context: c, localErr: errors.New("image response exceeds the allowed size"), imageProbeErrorCode: "IMAGE_PROBE_RESPONSE_LIMIT_EXCEEDED", newAPIError: types.NewError(errors.New("image response exceeds the allowed size"), types.ErrorCodeBadResponseBody)}
			}
			responseBody, readErr := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes+1))
			_ = httpResp.Body.Close()
			if readErr != nil {
				return testResult{context: c, localErr: errors.New("failed to read image edit response"), imageProbeErrorCode: "IMAGE_PROBE_RESPONSE_READ_FAILED", newAPIError: types.NewError(errors.New("failed to read image edit response"), types.ErrorCodeReadResponseBodyFailed)}
			}
			if int64(len(responseBody)) > maxResponseBytes {
				return testResult{context: c, localErr: errors.New("image edit response exceeds the allowed size"), imageProbeErrorCode: "IMAGE_PROBE_RESPONSE_LIMIT_EXCEEDED", newAPIError: types.NewError(errors.New("image edit response exceeds the allowed size"), types.ErrorCodeBadResponseBody)}
			}
			httpResp.Body = io.NopCloser(bytes.NewReader(responseBody))
		}
		if httpResp.StatusCode != http.StatusOK {
			if isImageProbe {
				return testResult{context: c, localErr: errors.New("image upstream request failed"), imageProbeErrorCode: imageProbeStatusErrorCode(httpResp.StatusCode), newAPIError: types.NewError(errors.New("image upstream request failed"), types.ErrorCodeBadResponse)}
			}
			err := service.RelayErrorHandler(c.Request.Context(), httpResp, true)
			common.SysError(fmt.Sprintf(
				"channel test bad response: channel_id=%d name=%s type=%d model=%s endpoint_type=%s status=%d err=%v",
				channel.Id,
				channel.Name,
				channel.Type,
				testModel,
				endpointType,
				httpResp.StatusCode,
				err,
			))
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError),
			}
		}
	}
	usageA, respErr := adaptor.DoResponse(c, httpResp, info)
	if respErr != nil {
		return testResult{
			context:             c,
			localErr:            respErr,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_RESPONSE_INVALID"),
			newAPIError:         respErr,
		}
	}
	usage, usageErr := coerceTestUsage(usageA, isStream, info.GetEstimatePromptTokens())
	if usageErr != nil {
		return testResult{
			context:             c,
			localErr:            usageErr,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_RESPONSE_INVALID"),
			newAPIError:         types.NewOpenAIError(usageErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
		}
	}
	result := w.Result()
	respBody, err := readTestResponseBody(result.Body, isStream)
	if err != nil {
		return testResult{
			context:             c,
			localErr:            err,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_RESPONSE_READ_FAILED"),
			newAPIError:         types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError),
		}
	}
	if bodyErr := validateTestResponseBody(respBody, isStream); bodyErr != nil {
		return testResult{
			context:             c,
			localErr:            bodyErr,
			imageProbeErrorCode: imageProbeErrorCodeFor(isImageProbe, "IMAGE_PROBE_RESPONSE_INVALID"),
			newAPIError:         types.NewOpenAIError(bodyErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
		}
	}
	if isImageProbe && !gjson.ValidBytes(respBody) {
		return testResult{context: c, localErr: errors.New("invalid image response"), imageProbeErrorCode: "IMAGE_PROBE_RESPONSE_INVALID", newAPIError: types.NewError(errors.New("invalid image response"), types.ErrorCodeBadResponseBody)}
	}
	if isImageProbe && !isValidImageEditTestResponse(respBody) {
		return testResult{context: c, localErr: errors.New("image response did not contain an image result"), imageProbeErrorCode: "IMAGE_PROBE_RESULT_MISSING", newAPIError: types.NewError(errors.New("image response did not contain an image result"), types.ErrorCodeBadResponseBody)}
	}
	info.SetEstimatePromptTokens(usage.PromptTokens)

	quota, tieredResult := settleTestQuota(info, priceData, usage)
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	other := buildTestLogOther(c, info, priceData, usage, tieredResult)
	if !hasProbeRequest {
		model.RecordConsumeLog(c, testUserID, model.RecordConsumeLogParams{
			ChannelId:        channel.Id,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			ModelName:        info.OriginModelName,
			TokenName:        "模型测试",
			Quota:            quota,
			Content:          "模型测试",
			UseTimeSeconds:   int(consumedTime),
			IsStream:         info.IsStream,
			Group:            info.UsingGroup,
			Other:            other,
		})
	}
	if !isImageProbe {
		common.SysLog(fmt.Sprintf("testing channel #%d, response: \n%s", channel.Id, string(respBody)))
	}
	return testResult{context: c, responseBody: respBody, upstreamBodyBytes: len(respBody), responseHeaders: httpResp.Header.Clone()}
}

func isValidImageEditTestResponse(responseBody []byte) bool {
	data := gjson.GetBytes(responseBody, "data")
	if !data.IsArray() || len(data.Array()) == 0 {
		return false
	}
	first := data.Array()[0]
	return strings.TrimSpace(first.Get("url").String()) != "" || strings.TrimSpace(first.Get("b64_json").String()) != ""
}

func attachTestBillingRequestInput(info *relaycommon.RelayInfo, request dto.Request) error {
	if info == nil {
		return nil
	}

	input, err := helper.BuildBillingExprRequestInputFromRequest(request, info.RequestHeaders)
	if err != nil {
		return err
	}
	info.BillingRequestInput = &input
	return nil
}

func settleTestQuota(info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage) (int, *billingexpr.TieredResult) {
	if usage != nil && info != nil && info.TieredBillingSnapshot != nil {
		isClaudeUsageSemantic := usage.UsageSemantic == "anthropic" || info.GetFinalRequestRelayFormat() == types.RelayFormatClaude
		usedVars := billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString)
		if ok, quota, result := service.TryTieredSettle(info, service.BuildTieredTokenParams(usage, isClaudeUsageSemantic, usedVars)); ok {
			return quota, result
		}
	}

	quota := 0
	if !priceData.UsePrice {
		quota = usage.PromptTokens + int(math.Round(float64(usage.CompletionTokens)*priceData.CompletionRatio))
		quota = int(math.Round(float64(quota) * priceData.ModelRatio))
		if priceData.ModelRatio != 0 && quota <= 0 {
			quota = 1
		}
		return quota, nil
	}

	return int(priceData.ModelPrice * common.QuotaPerUnit), nil
}

func buildTestLogOther(c *gin.Context, info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage, tieredResult *billingexpr.TieredResult) map[string]interface{} {
	other := service.GenerateTextOtherInfo(c, info, priceData.ModelRatio, priceData.GroupRatioInfo.GroupRatio, priceData.CompletionRatio,
		usage.PromptTokensDetails.CachedTokens, priceData.CacheRatio, priceData.ModelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		service.InjectTieredBillingInfo(other, info, tieredResult)
	}
	return other
}

func coerceTestUsage(usageAny any, isStream bool, estimatePromptTokens int) (*dto.Usage, error) {
	switch u := usageAny.(type) {
	case *dto.Usage:
		return u, nil
	case dto.Usage:
		return &u, nil
	case nil:
		if !isStream {
			return nil, errors.New("usage is nil")
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	default:
		if !isStream {
			return nil, fmt.Errorf("invalid usage type: %T", usageAny)
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	}
}

func readTestResponseBody(body io.ReadCloser, isStream bool) ([]byte, error) {
	defer func() { _ = body.Close() }()
	const maxStreamLogBytes = 8 << 10
	if isStream {
		return io.ReadAll(io.LimitReader(body, maxStreamLogBytes))
	}
	return io.ReadAll(body)
}

func detectErrorFromTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return nil
	}
	if message := detectErrorMessageFromJSONBytes(b); message != "" {
		return fmt.Errorf("upstream error: %s", message)
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if message := detectErrorMessageFromJSONBytes(payload); message != "" {
			return fmt.Errorf("upstream error: %s", message)
		}
	}

	return nil
}

func validateStreamTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return errors.New("stream response body is empty")
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		return nil
	}

	return errors.New("stream response body does not contain a valid stream event")
}

func validateTestResponseBody(respBody []byte, isStream bool) error {
	if bodyErr := detectErrorFromTestResponseBody(respBody); bodyErr != nil {
		return bodyErr
	}
	if isStream {
		return validateStreamTestResponseBody(respBody)
	}
	return nil
}

func shouldUseStreamForAutomaticChannelTest(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex
}

func detectErrorMessageFromJSONBytes(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	if jsonBytes[0] != '{' && jsonBytes[0] != '[' {
		return ""
	}
	errVal := gjson.GetBytes(jsonBytes, "error")
	if !errVal.Exists() || errVal.Type == gjson.Null {
		return ""
	}

	message := gjson.GetBytes(jsonBytes, "error.message").String()
	if message == "" {
		message = gjson.GetBytes(jsonBytes, "error.error.message").String()
	}
	if message == "" && errVal.Type == gjson.String {
		message = errVal.String()
	}
	if message == "" {
		message = errVal.Raw
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "upstream returned error payload"
	}
	return message
}

func buildTestRequest(model string, endpointType string, channel *model.Channel, isStream bool) dto.Request {
	testResponsesInput := json.RawMessage(`[{"role":"user","content":"hi"}]`)

	// 根据端点类型构建不同的测试请求
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeEmbeddings:
			// 返回 EmbeddingRequest
			return &dto.EmbeddingRequest{
				Model: model,
				Input: []any{"hello world"},
			}
		case constant.EndpointTypeImageGeneration:
			// 返回 ImageRequest
			return &dto.ImageRequest{
				Model:  model,
				Prompt: "a cute cat",
				N:      lo.ToPtr(uint(1)),
				Size:   "1024x1024",
			}
		case constant.EndpointTypeImageEdit:
			return &dto.ImageRequest{Model: model, Prompt: "Apply a subtle neutral edit.", N: lo.ToPtr(uint(1)), Size: "1024x1024", Quality: "low"}
		case constant.EndpointTypeJinaRerank:
			// 返回 RerankRequest
			return &dto.RerankRequest{
				Model:     model,
				Query:     "What is Deep Learning?",
				Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
				TopN:      lo.ToPtr(2),
			}
		case constant.EndpointTypeOpenAIResponse:
			// 返回 OpenAIResponsesRequest
			return &dto.OpenAIResponsesRequest{
				Model:  model,
				Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
				Stream: lo.ToPtr(isStream),
			}
		case constant.EndpointTypeOpenAIResponseCompact:
			// 返回 OpenAIResponsesCompactionRequest
			return &dto.OpenAIResponsesCompactionRequest{
				Model: model,
				Input: testResponsesInput,
			}
		case constant.EndpointTypeAnthropic, constant.EndpointTypeGemini, constant.EndpointTypeOpenAI:
			// 返回 GeneralOpenAIRequest
			maxTokens := uint(16)
			if constant.EndpointType(endpointType) == constant.EndpointTypeGemini {
				maxTokens = 3000
			}
			req := &dto.GeneralOpenAIRequest{
				Model:  model,
				Stream: lo.ToPtr(isStream),
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hi",
					},
				},
				MaxTokens: lo.ToPtr(maxTokens),
			}
			if isStream {
				req.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
			}
			return req
		}
	}

	// 自动检测逻辑（保持原有行为）
	if strings.Contains(strings.ToLower(model), "rerank") {
		return &dto.RerankRequest{
			Model:     model,
			Query:     "What is Deep Learning?",
			Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
			TopN:      lo.ToPtr(2),
		}
	}

	// 先判断是否为 Embedding 模型
	if strings.Contains(strings.ToLower(model), "embedding") ||
		strings.HasPrefix(model, "m3e") ||
		strings.Contains(model, "bge-") {
		// 返回 EmbeddingRequest
		return &dto.EmbeddingRequest{
			Model: model,
			Input: []any{"hello world"},
		}
	}

	// Responses compaction models (must use /v1/responses/compact)
	if strings.HasSuffix(model, ratio_setting.CompactModelSuffix) {
		return &dto.OpenAIResponsesCompactionRequest{
			Model: model,
			Input: testResponsesInput,
		}
	}

	// Responses-only models (e.g. codex series)
	if strings.Contains(strings.ToLower(model), "codex") {
		return &dto.OpenAIResponsesRequest{
			Model:  model,
			Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
			Stream: lo.ToPtr(isStream),
		}
	}

	// Chat/Completion 请求 - 返回 GeneralOpenAIRequest
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  model,
		Stream: lo.ToPtr(isStream),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}
	if isStream {
		testRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}

	if dto.IsOpenAIReasoningOModel(model) {
		testRequest.MaxCompletionTokens = lo.ToPtr(uint(16))
	} else if strings.Contains(model, "thinking") {
		if !strings.Contains(model, "claude") {
			testRequest.MaxTokens = lo.ToPtr(uint(50))
		}
	} else if strings.Contains(model, "gemini") {
		testRequest.MaxTokens = lo.ToPtr(uint(3000))
	} else {
		testRequest.MaxTokens = lo.ToPtr(uint(16))
	}

	return testRequest
}

func TestChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	//defer func() {
	//	if channel.ChannelInfo.IsMultiKey {
	//		go func() { _ = channel.SaveChannelInfo() }()
	//	}
	//}()
	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	isStream, _ := strconv.ParseBool(c.Query("stream"))
	if constant.EndpointType(endpointType) == constant.EndpointTypeImageEdit {
		if strings.TrimSpace(testModel) == "" || isStream || c.Query("confirm_paid_image_edit") != "true" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "image edit test requires an explicit model, stream=false, and confirmation", "time": 0.0, "error_code": types.ErrorCodeInvalidRequest})
			return
		}
	}
	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tik := time.Now()
	requestCtx := context.Background()
	if c.Request != nil {
		requestCtx = c.Request.Context()
	}
	if constant.EndpointType(endpointType) == constant.EndpointTypeImageEdit {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(requestCtx, 180*time.Second)
		defer cancel()
	}
	result := testChannel(requestCtx, channel, testUserID, testModel, endpointType, isStream)
	if result.localErr != nil {
		resp := gin.H{
			"success": false,
			"message": result.localErr.Error(),
			"time":    0.0,
		}
		if result.newAPIError != nil {
			resp["error_code"] = result.newAPIError.GetErrorCode()
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0
	if result.newAPIError != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    result.newAPIError.Error(),
			"time":       consumedTime,
			"error_code": result.newAPIError.GetErrorCode(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}

// channelTestSummary records the outcome of one channel test cycle so the
// system task can persist a per-run result for history.
type channelTestSummary struct {
	Tested    int `json:"tested"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Disabled  int `json:"disabled"`
	Enabled   int `json:"enabled"`
}

// performChannelTests runs the channel test loop synchronously, honoring ctx
// cancellation so a system-task runner that loses its lease stops promptly. When
// report is non-nil it is called after each channel with (processed, total) so
// the system task can surface progress.
func performChannelTests(ctx context.Context, channels []*model.Channel, testUserID int, allowDisable bool, report func(processed, total int)) channelTestSummary {
	summary := channelTestSummary{}
	var disableThreshold = int64(common.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // a impossible value
	}

	total := len(channels)
	for index, channel := range channels {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if report != nil {
			report(index, total) // channels completed before this one
		}
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		isChannelEnabled := channel.Status == common.ChannelStatusEnabled
		tik := time.Now()
		result := testChannel(ctx, channel, testUserID, "", "", shouldUseStreamForAutomaticChannelTest(channel))
		tok := time.Now()
		milliseconds := tok.Sub(tik).Milliseconds()
		if ctx != nil && ctx.Err() != nil {
			break
		}

		summary.Tested++

		shouldBanChannel := false
		newAPIError := result.newAPIError
		// request error disables the channel
		if newAPIError != nil {
			shouldBanChannel = service.ShouldDisableChannel(result.newAPIError)
		}

		// 当错误检查通过，才检查响应时间
		if common.AutomaticDisableChannelEnabled && !shouldBanChannel {
			if milliseconds > disableThreshold {
				err := fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
				newAPIError = types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusRequestTimeout)
				shouldBanChannel = true
			}
		}

		if newAPIError == nil {
			summary.Succeeded++
		} else {
			summary.Failed++
		}

		// disable channel
		if allowDisable && isChannelEnabled && shouldBanChannel && channel.GetAutoBan() {
			processChannelError(result.context, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
			summary.Disabled++
		}

		// enable channel
		if result.localErr == nil && !isChannelEnabled && service.ShouldEnableChannel(newAPIError, channel.Status) {
			service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
			summary.Enabled++
		}

		channel.UpdateResponseTime(milliseconds)
		if common.RequestInterval > 0 {
			if ctx == nil {
				time.Sleep(common.RequestInterval)
			} else {
				select {
				case <-ctx.Done():
					return summary
				case <-time.After(common.RequestInterval):
				}
			}
		}
	}
	if report != nil && (ctx == nil || ctx.Err() == nil) {
		report(total, total) // mark complete only when the full set was tested
	}
	return summary
}

// runChannelTestTask runs one synchronous channel test cycle for the system task
// runner (both the scheduled job and the manual "test all channels" trigger go
// through here). It honors ctx cancellation so a runner that loses its lease
// stops promptly. mode selects the channel set: an empty mode falls back to the
// configured monitor ChannelTestMode (scheduled behavior), while a manual
// trigger passes ChannelTestModeScheduledAll to test every channel. When notify
// is set the root user is notified on completion. Cross-instance execution is
// guarded by the system task per-type lock, so no process-local guard is needed.
func runChannelTestTask(ctx context.Context, mode string, notify bool, report func(processed, total int)) (channelTestSummary, error) {
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return channelTestSummary{}, err
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return channelTestSummary{}, err
	}
	if strings.TrimSpace(mode) == "" {
		mode = operation_setting.GetMonitorSetting().ChannelTestMode
	}
	selected := selectChannelsForAutomaticTest(channels, mode)
	allowDisable := mode != operation_setting.ChannelTestModePassiveRecovery
	summary := performChannelTests(ctx, selected, testUserID, allowDisable, report)
	if notify && (ctx == nil || ctx.Err() == nil) {
		service.NotifyRootUser(dto.NotifyTypeChannelTest, "通道测试完成", "所有通道测试已完成")
	}
	return summary, nil
}

func selectChannelsForAutomaticTest(channels []*model.Channel, mode string) []*model.Channel {
	selected := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		if mode == operation_setting.ChannelTestModePassiveRecovery && channel.Status != common.ChannelStatusAutoDisabled {
			continue
		}
		selected = append(selected, channel)
	}
	return selected
}

// TestAllChannels enqueues a channel_test system task instead of running the
// test loop inline. If any channel_test task is already active, the manual run is
// rejected so the caller does not mistake a scheduled run for this manual one.
func TestAllChannels(c *gin.Context) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelTest, channelTestTaskPayload{
		Mode:   operation_setting.ChannelTestModeScheduledAll,
		Notify: true,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "已有通道测试任务正在运行或等待中，不能启动本次手动任务",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
				"type":    task.Type,
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"task_id": task.TaskID,
			"status":  task.Status,
		},
	})
}
