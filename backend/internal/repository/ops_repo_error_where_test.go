package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpsErrorLogsWhere_QueryUsesQualifiedColumns(t *testing.T) {
	filter := &service.OpsErrorLogFilter{
		Query: "ACCESS_DENIED",
	}

	where, args := buildOpsErrorLogsWhere(filter)
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
	if !strings.Contains(where, "LOWER(COALESCE(e.request_id,'')) LIKE LOWER(?)") {
		t.Fatalf("where should include qualified request_id condition: %s", where)
	}
	if !strings.Contains(where, "LOWER(COALESCE(e.client_request_id,'')) LIKE LOWER(?)") {
		t.Fatalf("where should include qualified client_request_id condition: %s", where)
	}
	if !strings.Contains(where, "LOWER(COALESCE(e.error_message,'')) LIKE LOWER(?)") {
		t.Fatalf("where should include qualified error_message condition: %s", where)
	}
}

func TestBuildOpsErrorLogsWhere_UserQueryUsesExistsSubquery(t *testing.T) {
	filter := &service.OpsErrorLogFilter{
		UserQuery: "admin@",
	}

	where, args := buildOpsErrorLogsWhere(filter)
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
	if !strings.Contains(where, "EXISTS (SELECT 1 FROM users u WHERE u.id = e.user_id AND LOWER(COALESCE(u.email,'')) LIKE LOWER(?))") {
		t.Fatalf("where should include EXISTS user email condition: %s", where)
	}
}

func TestBuildOpsErrorLogsWhere_UsesEffectiveUpstreamStatus(t *testing.T) {
	where, _ := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{})

	const effectiveStatus = "COALESCE(NULLIF(e.upstream_status_code, 0), e.status_code, 0)"
	if !strings.Contains(where, effectiveStatus+" >= 400") {
		t.Fatalf("where should use upstream status for streaming errors: %s", where)
	}

	orderBy := opsErrorLogsOrderBy(&service.OpsErrorLogFilter{SortBy: "status_code"})
	if !strings.Contains(orderBy, effectiveStatus) {
		t.Fatalf("status sort should use effective upstream status: %s", orderBy)
	}
}
