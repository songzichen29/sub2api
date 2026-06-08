package handler

import (
	"errors"
	"net/http"
	"testing"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyReadErrorMessage_ClassifiesEncodingFailures(t *testing.T) {
	require.Equal(t,
		"Unsupported Content-Encoding: br",
		requestBodyReadErrorMessage(&pkghttputil.RequestBodyReadError{
			Kind:     pkghttputil.RequestBodyReadErrorKindUnsupportedEncoding,
			Encoding: "br",
			Err:      pkghttputil.ErrUnsupportedContentEncoding,
		}),
	)

	require.Equal(t,
		"Failed to decode request body with Content-Encoding: gzip",
		requestBodyReadErrorMessage(&pkghttputil.RequestBodyReadError{
			Kind:     pkghttputil.RequestBodyReadErrorKindDecode,
			Encoding: "gzip",
			Err:      errors.New("gzip: invalid header"),
		}),
	)

	require.Equal(t,
		"Failed to read request body",
		requestBodyReadErrorMessage(&pkghttputil.RequestBodyReadError{
			Kind: pkghttputil.RequestBodyReadErrorKindRead,
			Err:  errors.New("unexpected EOF"),
		}),
	)
}

func TestRequestBodyReadErrorStillExposesMaxBytesError(t *testing.T) {
	err := &pkghttputil.RequestBodyReadError{
		Kind: pkghttputil.RequestBodyReadErrorKindRead,
		Err:  &http.MaxBytesError{Limit: 16},
	}

	maxErr, ok := extractMaxBytesError(err)
	require.True(t, ok)
	require.Equal(t, int64(16), maxErr.Limit)
}
