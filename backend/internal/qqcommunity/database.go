package qqcommunity

// The upstream business layer uses numbered PostgreSQL SQL. Keep the dialect
// boundary inside this package: no site queries or drivers are intercepted.
import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

type database struct {
	raw       *sql.DB
	mysql     bool
	execError func(error)
}
type transaction struct {
	raw   *sql.Tx
	mysql bool
}
type row struct{ scan func(...any) error }

func (r *row) Scan(v ...any) error { return r.scan(v...) }
func errorRow(err error) *row      { return &row{scan: func(...any) error { return err }} }
func valueRow(v any) *row {
	return &row{scan: func(dst ...any) error {
		if len(dst) != 1 {
			return fmt.Errorf("expected one returning column")
		}
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, dst[0])
	}}
}
func wrapDatabase(db *sql.DB) *database {
	if db == nil {
		return nil
	}
	return &database{raw: db, mysql: strings.Contains(strings.ToLower(fmt.Sprintf("%T", db.Driver())), "mysql")}
}
func (d *database) BeginTx(ctx context.Context, opt *sql.TxOptions) (*transaction, error) {
	tx, err := d.raw.BeginTx(ctx, opt)
	if err != nil {
		return nil, err
	}
	if d.mysql && (opt == nil || !opt.ReadOnly) {
		// A single, existing InnoDB row replaces transaction advisory locks.
		// Transactions never hold this lock across model/QQ network requests.
		var id int
		if err = tx.QueryRowContext(ctx, "SELECT id FROM qqc_transaction_lock WHERE id=1 FOR UPDATE").Scan(&id); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return &transaction{raw: tx, mysql: d.mysql}, nil
}
func (t *transaction) Commit() error   { return t.raw.Commit() }
func (t *transaction) Rollback() error { return t.raw.Rollback() }

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func dbExec(ctx context.Context, e executor, mysql bool, q string, args ...any) (sql.Result, error) {
	if mysql && strings.Contains(q, "pg_advisory_xact_lock") {
		return e.ExecContext(ctx, "DO 0")
	}
	q, args, err := bindSQL(mysql, q, args)
	if err != nil {
		return nil, err
	}
	return e.ExecContext(ctx, q, args...)
}
func dbQuery(ctx context.Context, e executor, mysql bool, q string, args ...any) (*sql.Rows, error) {
	q, args, err := bindSQL(mysql, q, args)
	if err != nil {
		return nil, err
	}
	return e.QueryContext(ctx, q, args...)
}

var returningRE = regexp.MustCompile(`(?i)\s+RETURNING\s+(\w+)\s*$`)
var insertRE = regexp.MustCompile(`(?is)^\s*INSERT INTO (\w+)\s*\(([^)]+)\)\s*VALUES\s*\((.*?)\)\s*(?:ON CONFLICT|$)`)

func dbRow(ctx context.Context, e executor, mysql bool, q string, args ...any) *row {
	if mysql {
		if strings.HasPrefix(strings.TrimSpace(q), "SELECT pg_try_advisory_xact_lock(") {
			if _, ok := e.(*sql.Tx); !ok {
				return errorRow(fmt.Errorf("QQ advisory lock requires a transaction"))
			}
			// BeginTx already holds the cross-instance serialization row.
			return valueRow(true)
		}
		if m := returningRE.FindStringSubmatch(q); m != nil {
			q = returningRE.ReplaceAllString(q, "")
			if strings.HasPrefix(strings.TrimSpace(q), "DELETE ") {
				selectQ := regexp.MustCompile(`(?i)^DELETE FROM`).ReplaceAllString(q, "SELECT "+m[1]+" FROM")
				var v string
				if err := dbRow(ctx, e, true, selectQ, args...).Scan(&v); err != nil {
					return errorRow(err)
				}
				res, err := dbExec(ctx, e, true, q, args...)
				if err != nil {
					return errorRow(err)
				}
				n, err := res.RowsAffected()
				if err != nil {
					return errorRow(err)
				}
				if n == 0 {
					return errorRow(sql.ErrNoRows)
				}
				return valueRow(v)
			}
			res, err := dbExec(ctx, e, true, q, args...)
			if err != nil {
				return errorRow(err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return errorRow(err)
			}
			if n == 0 {
				return errorRow(sql.ErrNoRows)
			}
			if m[1] == "id" {
				id, err := res.LastInsertId()
				if err != nil {
					return errorRow(err)
				}
				return valueRow(id)
			}
			// Quota counters return the current value only if the bounded upsert
			// changed a row. SELECT runs on the same locked transaction.
			ins := insertRE.FindStringSubmatch(q)
			if ins == nil {
				return errorRow(fmt.Errorf("unsupported returning statement"))
			}
			cols, vals := splitSQL(ins[2], ','), splitSQL(ins[3], ',')
			where := []string{}
			a := []any{}
			for i, col := range cols {
				if strings.TrimSpace(col) == m[1] {
					continue
				}
				v := strings.TrimSpace(vals[i])
				if strings.HasPrefix(v, "$") {
					idx, _ := strconv.Atoi(v[1:])
					if idx > 0 && idx <= len(args) {
						a = append(a, args[idx-1])
						where = append(where, col+"=$"+strconv.Itoa(len(a)))
					}
				}
			}
			return dbRow(ctx, e, true, "SELECT "+m[1]+" FROM "+ins[1]+" WHERE "+strings.Join(where, " AND "), a...)
		}
	}
	q, args, err := bindSQL(mysql, q, args)
	if err != nil {
		return errorRow(err)
	}
	r := e.QueryRowContext(ctx, q, args...)
	return &row{scan: r.Scan}
}
func (d *database) ExecContext(c context.Context, q string, a ...any) (sql.Result, error) {
	r, err := dbExec(c, d.raw, d.mysql, q, a...)
	if err != nil && d.execError != nil {
		d.execError(fmt.Errorf("%s: %w", q, err))
	}
	return r, err
}
func (d *database) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	return dbQuery(c, d.raw, d.mysql, q, a...)
}
func (d *database) QueryRowContext(c context.Context, q string, a ...any) *row {
	return dbRow(c, d.raw, d.mysql, q, a...)
}
func (t *transaction) ExecContext(c context.Context, q string, a ...any) (sql.Result, error) {
	return dbExec(c, t.raw, t.mysql, q, a...)
}
func (t *transaction) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	return dbQuery(c, t.raw, t.mysql, q, a...)
}
func (t *transaction) QueryRowContext(c context.Context, q string, a ...any) *row {
	return dbRow(c, t.raw, t.mysql, q, a...)
}

// Split on a delimiter outside SQL strings and nested calls.
func splitSQL(s string, sep byte) []string {
	var out []string
	start, depth := 0, 0
	quoted := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			if quoted && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
		}
		if quoted {
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if s[i] == sep && depth == 0 {
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}
func closeParen(s string, start int) int {
	depth := 0
	quoted := false
	for i := start; i < len(s); i++ {
		if s[i] == '\'' {
			if quoted && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
		}
		if quoted {
			continue
		}
		if s[i] == '(' {
			depth++
		}
		if s[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

var paramRE = regexp.MustCompile(`\$(\d+)`)
var anyRE = regexp.MustCompile(`([\w.]+)\s*=\s*ANY\(\$(\d+)\)`)
var zoneRE = regexp.MustCompile(`\(([\w.]+) AT TIME ZONE 'Asia/Shanghai'\)::date`)
var dateTextRE = regexp.MustCompile(`(\$\d+|[A-Za-z_][\w.]*)::date::text`)
var dateCastTextRE = regexp.MustCompile(`(\$\d+|[A-Za-z_][\w.]*)::text`)
var castRE = regexp.MustCompile(`::(?:double precision|timestamptz|timestamp|numeric|jsonb|date|bigint|int|text)(?:\[\])?`)
var intervalRE = regexp.MustCompile(`INTERVAL '(\d+) (milliseconds|seconds?|minutes?|hours?|days?)'`)
var variableIntervalRE = regexp.MustCompile(`\(\$(\d+)\s*\*\s*INTERVAL '1 second'\)`)
var shareRE = regexp.MustCompile(`FOR (?:NO KEY )?UPDATE(?: OF \w+)?(?: FOR NO KEY UPDATE OF \w+)?|FOR SHARE(?: OF \w+)?`)
var keyRE = regexp.MustCompile(`\bkey\b`)
var rankRE = regexp.MustCompile(`\brank\b`)
var excludedRE = regexp.MustCompile(`(?i)EXCLUDED\.(\w+)`)
var conflictRE = regexp.MustCompile(`(?is) ON CONFLICT(?:\s*\([^)]*\))? DO (.*)$`)
var substrRE = regexp.MustCompile(`substring\((\w+) from (\d+)\)`)
var strposRE = regexp.MustCompile(`(?i)\bstrpos\(`)

func rewriteStrpos(q string) string {
	for start := 0; start < len(q); {
		m := strposRE.FindStringIndex(q[start:])
		if m == nil {
			break
		}
		a, open := start+m[0], start+m[1]-1
		end := closeParen(q, open)
		if end < 0 {
			break
		}
		args := splitSQL(q[open+1:end], ',')
		if len(args) != 2 {
			start = end + 1
			continue
		}
		replacement := "LOCATE(" + rewriteStrpos(args[1]) + "," + rewriteStrpos(args[0]) + ")"
		q = q[:a] + replacement + q[end+1:]
		start = a + len(replacement)
	}
	return q
}

func mysqlSQL(q string) string {
	q = normalizeSQL(q)
	q = mysqlRelations(q)
	q = strings.ReplaceAll(q, "AS NOT MATERIALIZED", "AS")
	q = regexp.MustCompile(`->>'(\w+)'`).ReplaceAllString(q, "->>'$$.$1'")
	q = zoneRE.ReplaceAllString(q, "DATE(DATE_ADD($1,INTERVAL 8 HOUR))")
	q = strings.ReplaceAll(q, "::text[]", "")
	q = dateTextRE.ReplaceAllString(q, "DATE_FORMAT($1,'%Y-%m-%d')")
	// VARCHAR casts preserve date-only results and numeric textual APIs.
	q = dateCastTextRE.ReplaceAllString(q, "CAST($1 AS CHAR)")
	q = strings.ReplaceAll(q, "clock_timestamp()", "NOW(6)")
	q = strings.ReplaceAll(q, "NOW()", "NOW(6)")
	q = strings.ReplaceAll(q, "NOW()+$11::interval", "DATE_ADD(NOW(), INTERVAL $11 SECOND)")
	q = castRE.ReplaceAllString(q, "")
	q = variableIntervalRE.ReplaceAllStringFunc(q, func(v string) string { return "INTERVAL $" + variableIntervalRE.FindStringSubmatch(v)[1] + " SECOND" })
	q = intervalRE.ReplaceAllStringFunc(q, func(s string) string {
		m := intervalRE.FindStringSubmatch(s)
		unit := strings.TrimSuffix(strings.ToUpper(m[2]), "S")
		if m[2] == "milliseconds" {
			v, _ := strconv.Atoi(m[1])
			return fmt.Sprintf("INTERVAL %d MICROSECOND", v*1000)
		}
		return "INTERVAL " + m[1] + " " + unit
	})
	q = anyRE.ReplaceAllString(q, `$1 IN (SELECT v FROM JSON_TABLE($$$2, '$$[*]' COLUMNS(v VARCHAR(191) PATH '$$')) qqc_array)`)
	q = strings.ReplaceAll(q, "CURRENT_DATE-2", "DATE_SUB(CURRENT_DATE, INTERVAL 2 DAY)")
	q = strings.ReplaceAll(q, "CURRENT_DATE-32", "DATE_SUB(CURRENT_DATE, INTERVAL 32 DAY)")
	q = shareRE.ReplaceAllStringFunc(q, func(s string) string {
		if strings.HasPrefix(s, "FOR SHARE") {
			return "FOR SHARE"
		}
		return "FOR UPDATE"
	})
	q = substrRE.ReplaceAllString(q, "SUBSTRING($1,$2)")
	q = rewriteStrpos(q)
	q = regexp.MustCompile(`(?i)([\w.]+) ILIKE '%'\|\|(\$\d+)\|\|'%'`).ReplaceAllString(q, "$1 LIKE CONCAT('%',$2,'%')")
	q = strings.ReplaceAll(q, " NULLS LAST", "")
	q = regexp.MustCompile(`([\w.]+)->'is_guaranteed'='true'`).ReplaceAllString(q, "JSON_EXTRACT($1,'$$.is_guaranteed')=TRUE")
	q = rewriteFilters(q)
	q = strings.ReplaceAll(q, "string_agg(DISTINCT NULLIF(o.error,''),'; ')", "GROUP_CONCAT(DISTINCT NULLIF(o.error,'') SEPARATOR '; ')")
	q = strings.ReplaceAll(q, "'user:'||m.message_id", "CONCAT('user:',m.message_id)")
	q = strings.ReplaceAll(q, "m.group_id||':'||m.member_id", "CONCAT(m.group_id,':',m.member_id)")
	q = strings.ReplaceAll(q, "'qq-growth:'||CAST(b.user_id AS CHAR)||':'||task_id", "CONCAT('qq-growth:',b.user_id,':',task_id)")
	q = strings.ReplaceAll(q, "unnest($4) task_id", "JSON_TABLE($4, '$[*]' COLUMNS(task_id VARCHAR(191) PATH '$')) qqc_tasks")
	if m := conflictRE.FindStringSubmatch(q); m != nil {
		base := q[:strings.Index(q, m[0])]
		action := m[1]
		if strings.EqualFold(strings.TrimSpace(action), "NOTHING") {
			op := strings.Index(base, "(")
			cl := strings.Index(base[op:], ",")
			if cl < 0 {
				cl = strings.Index(base[op:], ")")
			}
			col := strings.TrimSpace(base[op+1 : op+cl])
			q = base + " ON DUPLICATE KEY UPDATE " + col + "=" + col
		} else {
			action = strings.TrimPrefix(action, "UPDATE SET ")
			cond := ""
			if i := strings.Index(action, " WHERE "); i >= 0 {
				cond = action[i+7:]
				action = action[:i]
			}
			assign := splitSQL(action, ',')
			for i, v := range assign {
				idx := strings.Index(v, "=")
				col := strings.TrimSpace(v[:idx])
				rhs := v[idx+1:]
				if cond != "" {
					guard := "@qqc_apply"
					if i == 0 {
						guard = "(@qqc_apply := (" + cond + "))"
					}
					rhs = "IF(" + guard + "," + rhs + "," + col + ")"
				}
				assign[i] = col + "=" + rhs
			}
			q = base + " ON DUPLICATE KEY UPDATE " + strings.Join(assign, ",")
		}
		q = excludedRE.ReplaceAllString(q, "VALUES($1)")
	}
	q = keyRE.ReplaceAllString(q, "`key`")
	q = rankRE.ReplaceAllString(q, "`rank`")
	return q
}

func normalizeSQL(q string) string {
	var out strings.Builder
	quoted, space := false, false
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == '\'' {
			if quoted && i+1 < len(q) && q[i+1] == '\'' {
				out.WriteString("''")
				i++
				continue
			}
			quoted = !quoted
		}
		if !quoted && (c == ' ' || c == '\n' || c == '\r' || c == '\t') {
			space = true
			continue
		}
		if space && out.Len() > 0 {
			out.WriteByte(' ')
		}
		space = false
		out.WriteByte(c)
	}
	return out.String()
}

// Convert PostgreSQL aggregate FILTER clauses, preserving nested expressions.
func rewriteFilters(q string) string {
	re := regexp.MustCompile(`(?i)\b(COUNT|SUM|MAX)\(`)
	for start := 0; start < len(q); {
		m := re.FindStringSubmatchIndex(q[start:])
		if m == nil {
			break
		}
		a := start + m[0]
		open := start + m[1] - 1
		end := closeParen(q, open)
		if end < 0 {
			break
		}
		tail := end + 1
		for tail < len(q) && (q[tail] == ' ' || q[tail] == '\n' || q[tail] == '\t') {
			tail++
		}
		if !strings.HasPrefix(strings.ToUpper(q[tail:]), "FILTER") {
			start = end + 1
			continue
		}
		p := strings.Index(q[tail:], "(") + tail
		close := closeParen(q, p)
		if close < 0 {
			break
		}
		cond := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(q[p+1:close]), "WHERE"))
		fn := q[start+m[2] : start+m[3]]
		arg := q[open+1 : end]
		if strings.EqualFold(fn, "COUNT") && arg == "*" {
			arg = "1"
		}
		replacement := fn + "(CASE WHEN " + cond + " THEN " + arg + " ELSE NULL END)"
		q = q[:a] + replacement + q[close+1:]
		start = a + len(replacement)
	}
	return q
}
func bindSQL(mysql bool, q string, args []any) (string, []any, error) {
	if !mysql {
		return q, args, nil
	}
	q = mysqlSQL(q)
	var ordered []any
	var fail error
	// Only substitute parameter tokens outside literals (JSON paths contain $).
	quoted := false
	var out strings.Builder
	for i := 0; i < len(q); i++ {
		if q[i] == '\'' {
			if quoted && i+1 < len(q) && q[i+1] == '\'' {
				out.WriteString("''")
				i++
				continue
			}
			quoted = !quoted
		}
		if !quoted && q[i] == '$' && i+1 < len(q) && q[i+1] >= '0' && q[i+1] <= '9' {
			j := i + 1
			for j < len(q) && q[j] >= '0' && q[j] <= '9' {
				j++
			}
			n, _ := strconv.Atoi(q[i+1 : j])
			if n < 1 || n > len(args) {
				fail = fmt.Errorf("invalid QQ SQL parameter %d", n)
				break
			}
			v := args[n-1]
			if a, ok := v.(driver.Valuer); ok && !(reflect.ValueOf(a).Kind() == reflect.Ptr && reflect.ValueOf(a).IsNil()) {
				raw, err := a.Value()
				if err != nil {
					return "", nil, err
				}
				if s, ok := raw.(string); ok && strings.HasPrefix(s, "{") {
					var sa pq.StringArray
					if err = sa.Scan(s); err != nil {
						return "", nil, err
					}
					b, _ := json.Marshal([]string(sa))
					v = string(b)
				}
			}
			ordered = append(ordered, v)
			out.WriteByte('?')
			i = j - 1
			continue
		}
		out.WriteByte(q[i])
	}
	return out.String(), ordered, fail
}
