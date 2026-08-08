package channel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type formRequestTestAdaptor struct{ url string }

func (a formRequestTestAdaptor) Init(*relaycommon.RelayInfo) {}
func (a formRequestTestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}
func (a formRequestTestAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}
func (a formRequestTestAdaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, nil
}
func (a formRequestTestAdaptor) GetModelList() []string { return nil }
func (a formRequestTestAdaptor) GetChannelName() string { return "test" }
func (a formRequestTestAdaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, nil
}
func (a formRequestTestAdaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, nil
}

func TestDoFormRequestUsesMultipartAndRequestContext(t *testing.T) {
	service.InitHttpClient()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, []byte("multipart-body"), body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	ctx.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	resp, err := DoFormRequest(formRequestTestAdaptor{url: server.URL}, ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, bytes.NewBufferString("multipart-body"))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 1, requests)
}

func TestDoFormRequestCancellationDoesNotSendTwiceOrLeakSecrets(t *testing.T) {
	service.InitHttpClient()
	var requests atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		started <- struct{}{}
		<-release
	}))

	requestContext, cancel := context.WithCancel(context.Background())
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequestWithContext(requestContext, http.MethodPost, "/v1/images/edits", nil)
	ctx.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	ctx.Request.Header.Set("X-Secret-Header", "secret-header")
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	defer func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousWriter
		common.LogWriterMu.Unlock()
	}()
	result := make(chan error, 1)
	go func() {
		_, err := DoFormRequest(formRequestTestAdaptor{url: server.URL + "/secret-path"}, ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, bytes.NewBufferString("secret-body"))
		result <- err
	}()
	<-started
	cancel()
	err := <-result
	close(release)
	server.Close()
	require.Error(t, err)
	require.ErrorContains(t, err, "do request failed")
	var apiError *types.NewAPIError
	require.True(t, errors.As(err, &apiError))
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiError.GetErrorCode())
	require.NotContains(t, err.Error(), "secret-path")
	require.NotContains(t, err.Error(), "secret-body")
	require.Equal(t, int32(1), requests.Load())
	require.Contains(t, logs.String(), "upstream request failed")
	for _, secret := range []string{"secret-path", "secret-body", "secret-header", "127.0.0.1"} {
		require.NotContains(t, logs.String(), secret)
	}
}

func TestDoFormRequestDeadlineDoesNotSendTwiceOrLeakSecrets(t *testing.T) {
	service.InitHttpClient()
	var requests atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		started <- struct{}{}
		<-release
	}))

	requestContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequestWithContext(requestContext, http.MethodPost, "/v1/images/edits", nil)
	ctx.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	defer func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousWriter
		common.LogWriterMu.Unlock()
	}()
	result := make(chan error, 1)
	go func() {
		_, err := DoFormRequest(formRequestTestAdaptor{url: server.URL + "/secret-path"}, ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, bytes.NewBufferString("secret-body"))
		result <- err
	}()
	<-started
	err := <-result
	close(release)
	server.Close()
	require.Error(t, err)
	require.ErrorContains(t, err, "do request failed")
	var apiError *types.NewAPIError
	require.True(t, errors.As(err, &apiError))
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiError.GetErrorCode())
	require.NotContains(t, err.Error(), "secret-path")
	require.NotContains(t, err.Error(), "secret-body")
	require.Equal(t, int32(1), requests.Load())
	require.Contains(t, logs.String(), "upstream request failed")
	for _, secret := range []string{"secret-path", "secret-body", "127.0.0.1"} {
		require.NotContains(t, logs.String(), secret)
	}
}

func TestDoFormRequestTransportFailureDoesNotLeakSecretsOrRetry(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("closed listener received a request") }))
	closedURL := server.URL + "/secret-path?secret-query=1"
	server.Close()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	ctx.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	ctx.Request.Header.Set("X-Secret-Header", "secret-header")
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	defer func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousWriter
		common.LogWriterMu.Unlock()
	}()
	_, err := DoFormRequest(formRequestTestAdaptor{url: closedURL}, ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, bytes.NewBufferString("secret-body-and-prompt"))
	require.Error(t, err)
	var apiError *types.NewAPIError
	require.True(t, errors.As(err, &apiError))
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiError.GetErrorCode())
	require.ErrorContains(t, err, "do request failed")
	require.Contains(t, logs.String(), "upstream request failed")
	for _, secret := range []string{"secret-path", "secret-query", "secret-body", "secret-header", "127.0.0.1"} {
		require.NotContains(t, logs.String(), secret)
	}
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}
