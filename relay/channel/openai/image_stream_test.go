package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func imageStudioPNG(width, height uint32) string {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, int(width), int(height)))); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func imageStudioPNGWithLength(t *testing.T, width, height uint32, length int) string {
	t.Helper()
	bytes, err := base64.StdEncoding.DecodeString(imageStudioPNG(width, height))
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(append(bytes, make([]byte, length-len(bytes))...))
}

func TestOpenaiImageHandlerValidatesImageStudioPNGBeforeForwarding(t *testing.T) {
	for _, testCase := range []struct {
		name string
		size string
		png  string
		want bool
	}{
		{"square", "1024x1024", imageStudioPNG(1024, 1024), true},
		{"xiaohongshu", "1056x1408", imageStudioPNG(1056, 1408), true},
		{"landscape", "1536x864", imageStudioPNG(1536, 864), true},
		{"portrait", "864x1536", imageStudioPNG(864, 1536), true},
		{"mismatch", "1536x864", imageStudioPNG(1024, 1024), false},
		{"bad base64", "1024x1024", "not-base64", false},
		{"base64 CRLF", "1024x1024", imageStudioPNG(1024, 1024) + "\r\n", false},
		{"empty base64", "1024x1024", "", false},
		{"multiple images", "1024x1024", imageStudioPNG(1024, 1024), false},
		{"missing IHDR", "1024x1024", base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10}), false},
		{"header only", "1024x1024", imageStudioHeaderOnly(1024, 1024), false},
		{"missing IEND", "1024x1024", imageStudioWithoutIEND(t, 1024, 1024), false},
		{"bad compressed data", "1024x1024", imageStudioBadCompressedData(t, 1024, 1024), false},
		{"truncated IHDR", "1024x1024", imageStudioPNG(1024, 1024)[:32], false},
		{"bad CRC", "1024x1024", imageStudioPNGBadCRC(t, 1024, 1024), false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := `{"data":[{"b64_json":"` + testCase.png + `"}]}`
			if testCase.name == "multiple images" {
				body = `{"data":[{"b64_json":"` + testCase.png + `"},{"b64_json":"` + testCase.png + `"}]}`
			}
			c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
			c.Request.URL.Path = "/pg/images/generations"
			c.Set(imageStudioExpectedSizeContextKey, testCase.size)
			_, err := OpenaiImageHandler(c, info, resp)
			if testCase.want {
				require.Nil(t, err)
				require.Equal(t, body, recorder.Body.String())
			} else {
				require.NotNil(t, err)
				require.Empty(t, recorder.Body.String())
			}
		})
	}
}

func imageStudioHeaderOnly(width, height uint32) string {
	bytes, _ := base64.StdEncoding.DecodeString(imageStudioPNG(width, height))
	return base64.StdEncoding.EncodeToString(bytes[:33])
}

func imageStudioWithoutIEND(t *testing.T, width, height uint32) string {
	t.Helper()
	bytes, err := base64.StdEncoding.DecodeString(imageStudioPNG(width, height))
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(bytes[:len(bytes)-12])
}

func imageStudioBadCompressedData(t *testing.T, width, height uint32) string {
	t.Helper()
	imageBytes, err := base64.StdEncoding.DecodeString(imageStudioPNG(width, height))
	require.NoError(t, err)
	for offset := 8; offset+12 <= len(imageBytes); {
		chunkLength := int(binary.BigEndian.Uint32(imageBytes[offset : offset+4]))
		dataOffset := offset + 8
		crcOffset := dataOffset + chunkLength
		require.LessOrEqual(t, crcOffset+4, len(imageBytes))

		if string(imageBytes[offset+4:dataOffset]) == "IDAT" {
			require.Greater(t, chunkLength, 2)
			imageBytes[dataOffset+2] ^= 0xff
			binary.BigEndian.PutUint32(imageBytes[crcOffset:crcOffset+4], crc32.ChecksumIEEE(imageBytes[offset+4:crcOffset]))
			require.Equal(t, crc32.ChecksumIEEE(imageBytes[offset+4:crcOffset]), binary.BigEndian.Uint32(imageBytes[crcOffset:crcOffset+4]))
			_, err = png.Decode(bytes.NewReader(imageBytes))
			require.Error(t, err)
			return base64.StdEncoding.EncodeToString(imageBytes)
		}

		offset = crcOffset + 4
	}
	require.Fail(t, "IDAT chunk not found")
	return ""
}

func TestPNGDimensionsFromBase64HonorsExactDecodedLimitWithPadding(t *testing.T) {
	exactLimit := imageStudioPNGWithLength(t, 1024, 1024, imageStudioMaxDecodedBytes)
	_, err := pngDimensionsFromBase64(exactLimit, imageStudioDimensions{width: 1024, height: 1024})
	require.NoError(t, err)

	overLimit := imageStudioPNGWithLength(t, 1024, 1024, imageStudioMaxDecodedBytes+1)
	_, err = pngDimensionsFromBase64(overLimit, imageStudioDimensions{width: 1024, height: 1024})
	require.Error(t, err)
}

func imageStudioPNGBadCRC(t *testing.T, width, height uint32) string {
	t.Helper()
	bytes, err := base64.StdEncoding.DecodeString(imageStudioPNG(width, height))
	require.NoError(t, err)
	bytes[32] ^= 1
	return base64.StdEncoding.EncodeToString(bytes)
}

func TestValidateImageStudioResponseRejectsNonStringContext(t *testing.T) {
	c, _, _, _ := newImageTestContext(t, "", "application/json", false)
	c.Set(imageStudioExpectedSizeContextKey, 1024)
	err := validateImageStudioResponse(c, []byte(`{"data":[{"b64_json":"`+imageStudioPNG(1024, 1024)+`"}]}`))
	validationErr, ok := err.(*imageStudioValidationError)
	require.True(t, ok)
	require.Equal(t, imageStudioReasonExpectedSizeInvalid, validationErr.reason)
	require.Equal(t, "image studio response rejected: reason=EXPECTED_SIZE_INVALID expected=unknown actual=unknown", validationErr.safeLogMessage())
}

func TestValidateImageStudioResponseClassifiesSafeFailureReasons(t *testing.T) {
	validPNG := imageStudioPNG(1024, 1024)
	mismatchedPNG := imageStudioPNG(1536, 864)
	for _, testCase := range []struct {
		name       string
		body       string
		wantReason imageStudioValidationReason
		wantActual imageStudioDimensions
	}{
		{"image count mismatch", `{"data":[]}`, imageStudioReasonImageCountMismatch, imageStudioDimensions{}},
		{"missing b64", `{"data":[{}]}`, imageStudioReasonB64Missing, imageStudioDimensions{}},
		{"non-string b64", `{"data":[{"b64_json":1}]}`, imageStudioReasonB64Invalid, imageStudioDimensions{}},
		{"invalid b64", `{"data":[{"b64_json":"not-base64"}]}`, imageStudioReasonB64Invalid, imageStudioDimensions{}},
		{"invalid png", `{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("not a png payload that is long enough")) + `"}]}`, imageStudioReasonPNGInvalid, imageStudioDimensions{}},
		{"dimension mismatch", `{"data":[{"b64_json":"` + mismatchedPNG + `"}]}`, imageStudioReasonDimensionMismatch, imageStudioDimensions{width: 1536, height: 864}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			c, _, _, _ := newImageTestContext(t, "", "application/json", false)
			c.Set(imageStudioExpectedSizeContextKey, "1024x1024")

			err := validateImageStudioResponse(c, []byte(testCase.body))

			validationErr, ok := err.(*imageStudioValidationError)
			require.True(t, ok)
			require.Equal(t, testCase.wantReason, validationErr.reason)
			require.Equal(t, imageStudioDimensions{width: 1024, height: 1024}, validationErr.expected)
			require.Equal(t, testCase.wantActual, validationErr.actual)
			require.Equal(t, "invalid image studio response", validationErr.Error())
			require.NotContains(t, validationErr.safeLogMessage(), validPNG)
			require.NotContains(t, validationErr.safeLogMessage(), mismatchedPNG)
		})
	}
}

func TestOpenaiImageHandlerLogsOnlySafeImageStudioReasonAndDimensions(t *testing.T) {
	sensitiveMarkers := []string{
		"PROMPT_MARKER",
		"BASE64_MARKER",
		"BODY_MARKER",
		"TOKEN_MARKER",
		"COOKIE_MARKER",
		"OAUTH_MARKER",
		"UPSTREAM_MESSAGE_MARKER",
	}
	marker := strings.Join(sensitiveMarkers, "_")

	validPNG := imageStudioPNG(1024, 1024)
	for _, testCase := range []struct {
		name       string
		body       string
		expected   string
		wantReason string
		actual     string
	}{
		{
			name:       "expected size invalid",
			body:       `{"body_marker":"` + marker + `","data":[{"b64_json":"` + validPNG + `"}]}`,
			expected:   "unknown",
			wantReason: string(imageStudioReasonExpectedSizeInvalid),
			actual:     "unknown",
		},
		{
			name:       "image count mismatch",
			body:       `{"body_marker":"` + marker + `","data":[]}`,
			expected:   "1024x1024",
			wantReason: string(imageStudioReasonImageCountMismatch),
			actual:     "unknown",
		},
		{
			name:       "base64 missing",
			body:       `{"body_marker":"` + marker + `","data":[{"upstream_message":"` + marker + `"}]}`,
			expected:   "1024x1024",
			wantReason: string(imageStudioReasonB64Missing),
			actual:     "unknown",
		},
		{
			name:       "base64 invalid",
			body:       `{"body_marker":"` + marker + `","data":[{"b64_json":"BASE64_MARKER"}]}`,
			expected:   "1024x1024",
			wantReason: string(imageStudioReasonB64Invalid),
			actual:     "unknown",
		},
		{
			name:       "png invalid",
			body:       `{"body_marker":"` + marker + `","data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("not a png payload that is long enough")) + `"}]}`,
			expected:   "1024x1024",
			wantReason: string(imageStudioReasonPNGInvalid),
			actual:     "unknown",
		},
		{
			name:       "dimension mismatch",
			body:       `{"body_marker":"` + marker + `","data":[{"b64_json":"` + imageStudioPNG(1536, 864) + `"}]}`,
			expected:   "1024x1024",
			wantReason: string(imageStudioReasonDimensionMismatch),
			actual:     "1536x864",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var logs bytes.Buffer
			common.LogWriterMu.Lock()
			oldErrorWriter := gin.DefaultErrorWriter
			gin.DefaultErrorWriter = &logs
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultErrorWriter = oldErrorWriter
				common.LogWriterMu.Unlock()
			})

			c, recorder, resp, info := newImageTestContext(t, testCase.body, "application/json", false)
			c.Request.URL.Path = "/pg/images/generations"
			c.Request.Body = io.NopCloser(strings.NewReader("prompt=" + marker))
			c.Request.Header.Set("Authorization", "Bearer "+marker)
			c.Request.Header.Set("Cookie", marker)
			c.Request.Header.Set("X-OAuth-Marker", marker)
			if testCase.name == "expected size invalid" {
				c.Set(imageStudioExpectedSizeContextKey, marker)
			} else {
				c.Set(imageStudioExpectedSizeContextKey, "1024x1024")
			}

			_, apiErr := OpenaiImageHandler(c, info, resp)

			require.Error(t, apiErr)
			require.Empty(t, recorder.Body.String())
			logOutput := logs.String()
			logMessage := strings.TrimSpace(logOutput[strings.LastIndex(logOutput, "|")+1:])
			require.Equal(t, "image studio response rejected: reason="+testCase.wantReason+" expected="+testCase.expected+" actual="+testCase.actual, logMessage)
			for _, sensitiveMarker := range sensitiveMarkers {
				require.NotContains(t, logOutput, sensitiveMarker)
			}
			require.Contains(t, logOutput, "reason="+testCase.wantReason)
			require.Contains(t, logOutput, "expected="+testCase.expected)
			require.Contains(t, logOutput, "actual="+testCase.actual)
			require.NotContains(t, logOutput, "reason=UNKNOWN")
			require.NotContains(t, logOutput, "prompt")
			require.NotContains(t, logOutput, "base64")
			require.NotContains(t, logOutput, "body")
			require.NotContains(t, logOutput, "token")
			require.NotContains(t, logOutput, "cookie")
			require.NotContains(t, logOutput, "oauth")
			require.NotContains(t, logOutput, "upstream")
		})
	}
}

func TestOpenaiImageHandlerLeavesPublicImageResponsesUntouched(t *testing.T) {
	body := `{"data":[{"b64_json":"not-base64"},{"b64_json":"also-not-a-png"}]}`
	c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
	_, err := OpenaiImageHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, body, recorder.Body.String())
}

func newImageTestContext(t *testing.T, body, contentType string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		IsStream:    isStream,
	}
	return c, recorder, resp, info
}

func TestOpenaiImageDoResponseUsesInfoIsStream(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"created":1710000000,"data":[{"b64_json":"image"}]}`

	t.Run("non-stream response stays JSON", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
		info.RelayMode = relayconstant.RelayModeImagesGenerations

		usage, err := (&Adaptor{}).DoResponse(c, resp, info)

		require.Nil(t, err)
		require.NotNil(t, usage)
		require.Equal(t, body, recorder.Body.String())
	})

	t.Run("stream response converts JSON to SSE", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)
		info.RelayMode = relayconstant.RelayModeImagesGenerations

		usage, err := (&Adaptor{}).DoResponse(c, resp, info)

		require.Nil(t, err)
		require.NotNil(t, usage)
		require.Contains(t, recorder.Body.String(), `event: image_generation.completed`)
		require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	})
}

// TestOpenaiImageStreamHandlerForwardsSSEAndUsage covers the core SSE path:
// chunks are forwarded with rebuilt event lines, usage is extracted and
// normalized (input_tokens -> prompt_tokens with details), and [DONE] is
// re-emitted to the client.
func TestOpenaiImageStreamHandlerForwardsSSEAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`event: image_generation.partial_image`,
		`data: {"type":"image_generation.partial_image","b64_json":"partial"}`,
		``,
		`data: {"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newImageTestContext(t, body, "text/event-stream", true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.ImageTokens)
	require.Equal(t, 1, usage.PromptTokensDetails.TextTokens)
	require.Contains(t, recorder.Body.String(), `event: image_generation.partial_image`)
	require.Contains(t, recorder.Body.String(), `data: {"type":"image_generation.partial_image","b64_json":"partial"}`)
	require.Contains(t, recorder.Body.String(), `data: {"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}`)
	require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Equal(t, 3.0, info.PriceData.OtherRatios()["n"], "streams without completed events keep the requested count")
}

func TestOpenaiImageStreamHandlerUsesCompletedEventCount(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"image_generation.partial_image","partial_image_index":0,"b64_json":"partial"}`,
		``,
		`data: {"type":"image_generation.completed","b64_json":"first"}`,
		``,
		`data: {"type":"image_edit.completed","b64_json":"second","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	c, _, resp, info := newImageTestContext(t, body, "text/event-stream", true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 2.0, info.PriceData.OtherRatios()["n"])
}

// blockingBody serves one SSE chunk, then blocks until Close (the scanner's
// cleanup) and returns EOF — keeping the upstream "open" while the client-side
// disconnect is simulated elsewhere.
type blockingBody struct {
	mu     sync.Mutex
	sent   bool
	chunk  []byte
	closed chan struct{}
}

func (b *blockingBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	if !b.sent {
		b.sent = true
		n := copy(p, b.chunk)
		b.mu.Unlock()
		return n, nil
	}
	b.mu.Unlock()
	<-b.closed
	return 0, io.EOF
}

func (b *blockingBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

// cancelAfterWriter cancels the request context right after the payload
// containing needle has been written to the client, simulating a client that
// disconnects after receiving that event. Cancelling from the write side (not
// the upstream read side) makes the abort deterministic: the handler has
// already processed and counted the event when the disconnect fires.
type cancelAfterWriter struct {
	gin.ResponseWriter
	needle string
	cancel context.CancelFunc
	once   sync.Once
}

func (w *cancelAfterWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if strings.Contains(string(p), w.needle) {
		w.once.Do(w.cancel)
	}
	return n, err
}

func (w *cancelAfterWriter) WriteString(s string) (int, error) {
	n, err := io.WriteString(w.ResponseWriter, s)
	if strings.Contains(s, w.needle) {
		w.once.Do(w.cancel)
	}
	return n, err
}

func newDisconnectingImageStream(t *testing.T, sseBody, disconnectAfter string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	c, recorder, resp, info := newImageTestContext(t, "", "text/event-stream", true)
	ctx, cancel := context.WithCancel(c.Request.Context())
	t.Cleanup(cancel)
	c.Request = c.Request.WithContext(ctx)
	c.Writer = &cancelAfterWriter{ResponseWriter: c.Writer, needle: disconnectAfter, cancel: cancel}
	resp.Body = &blockingBody{
		chunk:  []byte(sseBody),
		closed: make(chan struct{}),
	}
	return c, recorder, resp, info
}

// TestOpenaiImageStreamHandlerClientDisconnectKeepsRequestedCount guards the
// billing invariant: completed-event counting must not lower the charge when
// the client aborts the stream. Upstream already generated (and charged for)
// all requested images, so a disconnect after the first completed event keeps
// the requested n instead of dropping it to 1.
func TestOpenaiImageStreamHandlerClientDisconnectKeepsRequestedCount(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"first\"}\n\n"
	c, recorder, resp, info := newDisconnectingImageStream(t, body, "first")
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	// A client abort surfaces as client_gone (main-loop ctx watch) or
	// handler_stop (failed client write); both must be treated as untrusted.
	require.Contains(t,
		[]relaycommon.StreamEndReason{relaycommon.StreamEndReasonClientGone, relaycommon.StreamEndReasonHandlerStop},
		info.StreamStatus.EndReason)
	require.Contains(t, recorder.Body.String(), `"b64_json":"first"`)
	require.Equal(t, 3.0, info.PriceData.OtherRatios()["n"], "client abort must not reduce the billed image count")
}

// TestOpenaiImageStreamHandlerClientDisconnectRaisesCount covers the other
// direction of the abort guard: when completed events already exceed the
// recorded n, the higher actual count is billed even though the client aborted.
func TestOpenaiImageStreamHandlerClientDisconnectRaisesCount(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"image_generation.completed","b64_json":"first"}`,
		``,
		`data: {"type":"image_generation.completed","b64_json":"second"}`,
		``,
		``,
	}, "\n")
	c, _, resp, info := newDisconnectingImageStream(t, body, "second")
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 1)

	usage, err := OpenaiImageStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	require.Contains(t,
		[]relaycommon.StreamEndReason{relaycommon.StreamEndReasonClientGone, relaycommon.StreamEndReasonHandlerStop},
		info.StreamStatus.EndReason)
	require.Equal(t, 2.0, info.PriceData.OtherRatios()["n"], "completed events beyond the recorded n must raise the charge even on abort")
}

// TestOpenaiImageStreamHandlerWrapsJSONResponse covers the non-SSE fallback:
// a JSON upstream response is wrapped into pseudo-SSE completed events.
func TestOpenaiImageStreamHandlerWrapsJSONResponse(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"created":1710000000,"data":[{"b64_json":"first","revised_prompt":"draw a cat"},{"b64_json":"second"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}`

	c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.ImageTokens)
	require.Equal(t, 1, usage.PromptTokensDetails.TextTokens)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Contains(t, recorder.Body.String(), `event: image_generation.completed`)
	require.Contains(t, recorder.Body.String(), `"type":"image_generation.completed"`)
	require.Contains(t, recorder.Body.String(), `"b64_json":"first"`)
	require.Contains(t, recorder.Body.String(), `"b64_json":"second"`)
	require.Contains(t, recorder.Body.String(), `"revised_prompt":"draw a cat"`)
	require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	require.Equal(t, 2, strings.Count(recorder.Body.String(), `event: image_generation.completed`))
	require.Equal(t, 2.0, info.PriceData.OtherRatios()["n"])
}

func TestOpenaiImageHandlerUsesPositiveActualCountForFixedPrice(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	longImage := strings.Repeat("a", 4096)

	tests := []struct {
		name      string
		body      string
		usePrice  bool
		wantCount float64
	}{
		{
			name:      "fixed price uses data length",
			body:      `{"data":[{"b64_json":"` + longImage + `"},{"b64_json":"second"}]}`,
			usePrice:  true,
			wantCount: 2,
		},
		{
			name:      "empty data keeps requested count",
			body:      `{"data":[]}`,
			usePrice:  true,
			wantCount: 3,
		},
		{
			name:      "ratio billing ignores data length",
			body:      `{"data":[{"b64_json":"first"},{"b64_json":"second"}]}`,
			usePrice:  false,
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, resp, info := newImageTestContext(t, tt.body, "application/json", false)
			info.PriceData.UsePrice = tt.usePrice
			info.PriceData.AddOtherRatio("n", 3)

			_, err := OpenaiImageHandler(c, info, resp)

			require.Nil(t, err)
			require.Equal(t, tt.wantCount, info.PriceData.OtherRatios()["n"])
			require.Equal(t, tt.body, recorder.Body.String())
		})
	}
}

// TestOpenaiImageHandlersReturnJSONError covers JSON error responses for both
// entry points: the non-streaming handler and the stream handler's non-SSE
// fallback. Neither must leak the error body to the client.
func TestOpenaiImageHandlersReturnJSONError(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"error":{"message":"content moderation failed","type":"upstream_error","code":"content_moderation_failed","status":502}}`

	t.Run("non-streaming handler", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)

		usage, err := OpenaiImageHandler(c, info, resp)
		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, http.StatusOK, err.StatusCode)
		oaiError := err.ToOpenAIError()
		require.Equal(t, "content moderation failed", oaiError.Message)
		require.Equal(t, "upstream_error", oaiError.Type)
		require.Equal(t, "content_moderation_failed", oaiError.Code)
		require.Empty(t, recorder.Body.String())
	})

	t.Run("stream handler JSON fallback", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)

		usage, err := OpenaiImageStreamHandler(c, info, resp)
		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, http.StatusOK, err.StatusCode)
		require.Equal(t, "content moderation failed", err.ToOpenAIError().Message)
		require.Empty(t, recorder.Body.String())
	})

	t.Run("stream handler non-2xx stays JSON error", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)
		resp.StatusCode = http.StatusBadGateway

		usage, err := OpenaiImageStreamHandler(c, info, resp)
		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, http.StatusBadGateway, err.StatusCode)
		require.Equal(t, "content moderation failed", err.ToOpenAIError().Message)
		require.Empty(t, recorder.Body.String())
		require.NotContains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	})
}

// TestOpenaiImageStreamHandlerRecordsUpstreamErrorEvent verifies that an error
// event inside the SSE stream is recorded as a soft error while the payload is
// still forwarded to the client.
func TestOpenaiImageStreamHandlerRecordsUpstreamErrorEvent(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`event: image_generation.partial_image`,
		`data: {"type":"image_generation.partial_image","b64_json":"partial"}`,
		``,
		`event: error`,
		`data: {"type":"upstream_error","error":{"message":"stream error: stream ID 77; INTERNAL_ERROR; received from peer"}}`,
		``,
	}, "\n")

	c, recorder, resp, info := newImageTestContext(t, body, "text/event-stream", true)

	usage, err := OpenaiImageStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	require.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	require.True(t, info.StreamStatus.HasErrors())
	require.Equal(t, 1, info.StreamStatus.TotalErrorCount())
	require.Contains(t, info.StreamStatus.Errors[0].Message, "INTERNAL_ERROR")
	// The scanner strips the upstream "event: error" line; the event name is
	// rebuilt from the JSON "type" field (upstream_error). The error message
	// is still forwarded in the data: payload (stream ID 77).
	require.Contains(t, recorder.Body.String(), `event: upstream_error`)
	require.Contains(t, recorder.Body.String(), `stream ID 77`)
}
