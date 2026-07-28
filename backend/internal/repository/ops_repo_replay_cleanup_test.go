package repository

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestOpsErrorLogInsertDoesNotPersistRequestReplayFields(t *testing.T) {
	disallowedColumns := []string{
		"request_body",
		"request_headers",
		"request_body_truncated",
		"request_body_bytes",
		"is_retryable",
		"retry_count",
		"resolved_retry_id",
	}

	insertSQL := strings.ToLower(insertOpsErrorLogSQL)
	for _, column := range disallowedColumns {
		if strings.Contains(insertSQL, column) {
			t.Fatalf("ops error log insert still references dropped replay column %q", column)
		}
	}

	inputType := reflect.TypeOf(service.OpsInsertErrorLogInput{})
	disallowedFields := []string{
		"RequestBodyJSON",
		"RequestBodyTruncated",
		"RequestBodyBytes",
		"RequestHeadersJSON",
		"IsRetryable",
		"RetryCount",
		"ResolvedRetryID",
	}
	for _, field := range disallowedFields {
		if _, ok := inputType.FieldByName(field); ok {
			t.Fatalf("OpsInsertErrorLogInput still carries replay field %q", field)
		}
	}
}

func TestOpsErrorLogInsertSQLArityMatchesArguments(t *testing.T) {
	const columnsPrefix = "INSERT INTO ops_error_logs ("
	const valuesMarker = ") VALUES ("

	columnsStart := strings.Index(insertOpsErrorLogSQL, columnsPrefix)
	valuesStart := strings.Index(insertOpsErrorLogSQL, valuesMarker)
	if columnsStart < 0 || valuesStart < 0 || valuesStart <= columnsStart+len(columnsPrefix) {
		t.Fatalf("unexpected ops error log insert SQL shape")
	}

	columnCount := 0
	for _, column := range strings.Split(insertOpsErrorLogSQL[columnsStart+len(columnsPrefix):valuesStart], ",") {
		if strings.TrimSpace(column) != "" {
			columnCount++
		}
	}
	placeholderCount := strings.Count(insertOpsErrorLogSQL[valuesStart+len(valuesMarker):], "?")
	argumentCount := len(opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{}))

	if columnCount != placeholderCount || placeholderCount != argumentCount {
		t.Fatalf("ops error log insert arity mismatch: columns=%d placeholders=%d arguments=%d", columnCount, placeholderCount, argumentCount)
	}
}
