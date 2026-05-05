package repository

import (
	"reflect"
	"testing"
)

// 锁死「使用记录」筛选时使用的 SQL 表达式：必须是 display 维度
// (COALESCE(NULLIF(TRIM(requested_model), ''), model))，与 DTO 给前端的 Model 字段一致。
// 历史 bug：早期只过滤裸 `model` 列 → 用户在表格看到 requested_model（如 gpt-5.4）
// 但搜索时只能命中 model 列恰好等于该值的行，多数行被漏掉。

const expectedDisplayExpr = "COALESCE(NULLIF(TRIM(requested_model), ''), model)"

func TestAppendUsageLogModelWhereCondition_NonEmpty(t *testing.T) {
	conditions := []string{"user_id = ?"}
	args := []any{int64(7)}

	conditions, args = appendUsageLogModelWhereCondition(conditions, args, "gpt-5.4")

	wantCond := []string{"user_id = ?", expectedDisplayExpr + " = ?"}
	wantArgs := []any{int64(7), "gpt-5.4"}

	if !reflect.DeepEqual(conditions, wantCond) {
		t.Fatalf("conditions mismatch:\nhave %v\nwant %v", conditions, wantCond)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args mismatch:\nhave %v\nwant %v", args, wantArgs)
	}
}

func TestAppendUsageLogModelWhereCondition_EmptyOrWhitespace(t *testing.T) {
	cases := []string{"", "   ", "\t\n"}
	for _, in := range cases {
		conditions := []string{"user_id = ?"}
		args := []any{int64(7)}

		gotConds, gotArgs := appendUsageLogModelWhereCondition(conditions, args, in)

		if !reflect.DeepEqual(gotConds, []string{"user_id = ?"}) {
			t.Fatalf("conditions should be untouched for input %q, got %v", in, gotConds)
		}
		if !reflect.DeepEqual(gotArgs, []any{int64(7)}) {
			t.Fatalf("args should be untouched for input %q, got %v", in, gotArgs)
		}
	}
}

func TestAppendUsageLogModelQueryFilter_NonEmpty(t *testing.T) {
	query := "SELECT 1 FROM usage_logs WHERE user_id = ?"
	args := []any{int64(7)}

	gotQuery, gotArgs := appendUsageLogModelQueryFilter(query, args, "gpt-5.4")

	wantQuery := "SELECT 1 FROM usage_logs WHERE user_id = ? AND " + expectedDisplayExpr + " = ?"
	wantArgs := []any{int64(7), "gpt-5.4"}

	if gotQuery != wantQuery {
		t.Fatalf("query mismatch:\nhave %q\nwant %q", gotQuery, wantQuery)
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args mismatch:\nhave %v\nwant %v", gotArgs, wantArgs)
	}
}

func TestAppendUsageLogModelQueryFilter_EmptyOrWhitespace(t *testing.T) {
	cases := []string{"", "   ", "\t\n"}
	for _, in := range cases {
		query := "SELECT 1 FROM usage_logs WHERE user_id = ?"
		args := []any{int64(7)}

		gotQuery, gotArgs := appendUsageLogModelQueryFilter(query, args, in)

		if gotQuery != "SELECT 1 FROM usage_logs WHERE user_id = ?" {
			t.Fatalf("query should be untouched for input %q, got %q", in, gotQuery)
		}
		if !reflect.DeepEqual(gotArgs, []any{int64(7)}) {
			t.Fatalf("args should be untouched for input %q, got %v", in, gotArgs)
		}
	}
}

// 验证 display 表达式与 resolveModelDimensionExpression 的 default 分支保持同步——
// 任何一方调整都必须同步另一方，避免显示列和筛选条件再次脱节。
func TestUsageLogDisplayModelExpressionMatchesDimensionDefault(t *testing.T) {
	if usageLogDisplayModelExpression != resolveModelDimensionExpression("") {
		t.Fatalf(
			"筛选维度脱节：filter=%q resolveDimension(default)=%q",
			usageLogDisplayModelExpression,
			resolveModelDimensionExpression(""),
		)
	}
}
