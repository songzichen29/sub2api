package repository

import "strings"

func replaceSQLNowWithClock(sqlText string) string {
	return strings.ReplaceAll(sqlText, "NOW()", "clock.now_ts")
}
