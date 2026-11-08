package qqcommunity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func animatedOutputFixture(t *testing.T, count int) []byte {
	t.Helper()
	animation := &gif.GIF{LoopCount: 3}
	for i := 0; i < count; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Transparent, color.White})
		frame.SetColorIndex(i%8, 0, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 7+i)
		animation.Disposal = append(animation.Disposal, gif.DisposalBackground)
	}
	var data bytes.Buffer
	require.NoError(t, gif.EncodeAll(&data, animation))
	return data.Bytes()
}

func TestExplicitImageFormatsPreserveAnimationAndDefaults(t *testing.T) {
	data := animatedOutputFixture(t, 3)
	preserved, err := normalizeGeneratedImage(data, "gif")
	require.NoError(t, err)
	require.Equal(t, data, preserved, "retain timing, frames, disposal and loop bytes")
	legacy, err := normalizeGeneratedImage(data, "")
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", generatedImageType(legacy))
	_, err = normalizeGeneratedImage(legacy, "gif")
	require.Error(t, err)
	require.NotEmpty(t, imageOutputMessage(err))
	for i := 0; i < len(data); i++ {
		require.Error(t, validateGIFBudget(data[:i]), "truncated at %d", i)
	}
	_, err = normalizeGeneratedImage(animatedOutputFixture(t, maxGIFFrames+1), "gif")
	require.Error(t, err)
	_, err = normalizeGeneratedImage(append(data, make([]byte, maxGeneratedImageBytes)...), "gif")
	require.Error(t, err)
	c, prompt := imageCommandOptions(DefaultConfig(), "--gif waving cat")
	require.Equal(t, "gif", c.ImageOutputFormat)
	require.Equal(t, "waving cat", prompt)
	static, prompt := imageCommandOptions(c, "--static a cat")
	require.Equal(t, "png", static.ImageOutputFormat)
	require.Equal(t, "a cat", prompt)
	c, prompt = imageCommandOptions(DefaultConfig(), "a GIF logo")
	require.Empty(t, c.ImageOutputFormat)
	require.Equal(t, "a GIF logo", prompt)
}

func TestImageFormatRoundTripThroughGenerationAndEditing(t *testing.T) {
	data := animatedOutputFixture(t, 2)
	for _, edit := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if edit {
				require.Equal(t, "/v1/images/edits", r.URL.Path)
				require.NoError(t, r.ParseMultipartForm(1<<20))
				defer func() { require.NoError(t, r.MultipartForm.RemoveAll()) }()
				require.Equal(t, "gif", r.FormValue("output_format"))
			} else {
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "gif", body["output_format"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
		}))
		c := DefaultConfig()
		c.BaseURL, c.APIKey, c.ImageModel, c.ImageOutputFormat = server.URL+"/v1", "test-key", "test-image", "gif"
		s := New(nil, nil, false, nil)
		var result []byte
		var err error
		if edit {
			source, e := inputImageDataURL(data)
			require.NoError(t, e)
			result, err = s.editImage(context.Background(), c, "animate", source)
		} else {
			result, err = s.generateImage(context.Background(), c, "animate")
		}
		server.Close()
		require.NoError(t, err)
		require.Equal(t, data, result)
		require.Equal(t, 1, calls)
	}
}

func TestRuntimeImageFormatStaysWithQueuedEvent(t *testing.T) {
	input := runtimeToolRequest{Tool: "generate_image", GroupID: "123", MemberID: "456", MessageID: "789", Prompt: "a cat", OutputFormat: "gif"}
	event, err := runtimeImageEvent(input)
	require.NoError(t, err)
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	var decoded Event
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, "gif", decoded.RuntimeImageFormat)
	input.OutputFormat = "mp4"
	_, err = runtimeImageEvent(input)
	require.Error(t, err)
}
