package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderedAccountAPIKeysBatchResourcesUseOwningKey(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Goog-Api-Key")
		calls = append(calls, r.Method+" "+r.URL.Path+" "+key)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/upload/v1beta/files":
			_, _ = fmt.Fprintf(w, `{"file":{"name":"files/%s"}}`, key)
		case r.URL.Path == "/v1beta/models/gemini-test:batchGenerateContent":
			if key == "first" {
				w.WriteHeader(429)
				_, _ = w.Write([]byte(`{"error":{"message":"quota exhausted"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"name":"batches/second","state":"JOB_STATE_PENDING"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1beta/files/first":
			w.WriteHeader(204)
		case r.URL.Path == "/v1beta/batches/second":
			if key != "second" {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(`{"name":"batches/second","state":"JOB_STATE_RUNNING"}`))
		case r.URL.Path == "/v1beta/batches/second:cancel":
			if key != "second" {
				w.WriteHeader(404)
				return
			}
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	provider := NewGeminiAPIBatchImageProvider(NewGeminiBatchHTTPClient(server.URL, server.Client()))
	_, account := keyPoolRequest(t)
	account.Platform = PlatformGemini
	job, err := provider.Submit(context.Background(), nil, account, BatchImageInput{BatchID: "job", Model: "gemini-test", Items: []BatchImageInputItem{{CustomID: "item", Prompt: "test"}}})
	require.NoError(t, err)
	require.Equal(t, "files/second", job.ProviderInputRef)
	require.Equal(t, []string{
		"POST /upload/v1beta/files first", "POST /v1beta/models/gemini-test:batchGenerateContent first",
		"DELETE /v1beta/files/first first", "POST /upload/v1beta/files second", "POST /v1beta/models/gemini-test:batchGenerateContent second",
	}, calls)
	calls = nil
	saved := &BatchImageJob{ProviderJobName: &job.ProviderJobName}
	status, err := provider.Get(context.Background(), saved, account)
	require.NoError(t, err)
	require.Equal(t, BatchProviderStateRunning, status.InternalState)
	require.NoError(t, provider.Cancel(context.Background(), saved, account))
	require.Equal(t, []string{"GET /v1beta/batches/second first", "GET /v1beta/batches/second second", "POST /v1beta/batches/second:cancel first", "POST /v1beta/batches/second:cancel second"}, calls)
}
