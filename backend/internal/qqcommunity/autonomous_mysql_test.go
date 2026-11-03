package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// This exercises real row locks, SQL translation, delayed claims and atomic
// quotas against schema-only qqc_test. Never point it at production data.
func TestAutonomousMySQLBurstMediaAndQuota(t *testing.T) {
	dsn := os.Getenv("QQC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qqc_test", cfg.DBName)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	s := New(db, fixtureEncryptor{}, true, nil)
	ctx := context.Background()
	c := DefaultConfig()
	c.Enabled = true
	c.AppID = "qq-autonomous-integration"
	c.AllowedGroups = []string{"group"}
	c.Autonomous.Enabled = true
	c.Autonomous.Groups = c.AllowedGroups
	c.Autonomous.CooldownSeconds = 5
	c.Autonomous.MemberCooldownSeconds = 5
	c.Autonomous.MinMessages = 1
	c.Autonomous.SamplePercent = 100
	c.Autonomous.StartHour = 0
	c.Autonomous.EndHour = 0
	c.Autonomous.MediaEnabled = true
	c.Autonomous.MediaSamplePercent = 100
	c.Autonomous.MediaDailyCalls = 1
	raw, _ := json.Marshal(c)
	_, err = db.Exec("INSERT INTO settings (`key`,value,updated_at) VALUES('qqc_config',?,NOW(6)) ON DUPLICATE KEY UPDATE value=VALUES(value),updated_at=NOW(6)", string(raw))
	require.NoError(t, err)
	_, err = db.Exec("INSERT IGNORE INTO qqc_transaction_lock(id) VALUES(1)")
	require.NoError(t, err)
	for _, table := range []string{"qqc_jobs", "qqc_autonomous_state", "qqc_autonomous_seen", "qqc_autonomous_decisions", "qqc_daily_usage"} {
		_, err = db.Exec("DELETE FROM "+table+" WHERE app_id=?", c.AppID)
		require.NoError(t, err)
	}
	route := func(id, text string, media bool) {
		e := Event{ID: id, GroupID: "group", Content: text}
		e.Author.MemberID = "member"
		if media {
			e.Attachments = []Attachment{{URL: "https://qq.example/" + id + ".gif", ContentType: "image/gif", FileID: strings.TrimPrefix(id, "repeat-")}}
		}
		b, _ := json.Marshal(e)
		require.NoError(t, s.routeAutonomous(ctx, c, Payload{Type: groupMessageCreate, Data: b}, e, true))
	}
	complete := func(j Job) {
		_, err = db.Exec("UPDATE qqc_jobs SET status='completed' WHERE id=?", j.ID)
		require.NoError(t, err)
	}
	clearCooldown := func() {
		_, err = db.Exec("UPDATE qqc_jobs SET reply_reserved_at=DATE_SUB(NOW(6),INTERVAL 7 SECOND) WHERE app_id=? AND reply_reserved_at IS NOT NULL", c.AppID)
		require.NoError(t, err)
		_, err = db.Exec("UPDATE qqc_autonomous_state SET last_reply_at=DATE_SUB(NOW(6),INTERVAL 7 SECOND) WHERE app_id=?", c.AppID)
		require.NoError(t, err)
	}
	route("one", "你好", false)
	one, err := s.claimJob(ctx)
	require.NoError(t, err)
	require.Equal(t, "running", one.Status, "claimed jobs must finish normally instead of appearing deferred")
	require.Equal(t, "one", one.MessageID)
	route("two", "接下来怎么弄？", false)
	_, err = s.claimJob(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows, "one autonomous model per group")
	_, reason, err := s.reserveAutonomous(ctx, c, one, false)
	require.NoError(t, err)
	require.Empty(t, reason)
	_, reason, err = s.reserveAutonomous(ctx, c, one, true)
	require.NoError(t, err)
	require.Empty(t, reason)
	one.ShareHistory = true
	require.NoError(t, s.recordAutonomousReply(ctx, c, one))
	complete(one)
	two, err := s.claimJob(ctx)
	require.NoError(t, err)
	require.NoError(t, s.autonomousMessage(ctx, c, &two, Event{}))
	require.Equal(t, "queued", two.Status)
	_, err = s.claimJob(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows, "future not-before timestamp honored")
	clearCooldown()
	_, err = db.Exec("UPDATE qqc_jobs SET started_at=DATE_SUB(NOW(6),INTERVAL 1 SECOND) WHERE id=?", two.ID)
	require.NoError(t, err)
	two, err = s.claimJob(ctx)
	require.NoError(t, err)
	require.Equal(t, "two", two.MessageID)
	_, reason, err = s.reserveAutonomous(ctx, c, two, false)
	require.NoError(t, err)
	require.Empty(t, reason, "prior answer does not consume queued message gap")
	complete(two)
	route("media-one", "", true)
	media, err := s.claimJob(ctx)
	require.NoError(t, err)
	require.Equal(t, "media", media.TriggerReason)
	_, reason, err = s.reserveAutonomous(ctx, c, media, false)
	require.NoError(t, err)
	require.Empty(t, reason)
	complete(media)
	route("media-two", "", true)
	media, err = s.claimJob(ctx)
	require.NoError(t, err)
	_, reason, err = s.reserveAutonomous(ctx, c, media, false)
	require.NoError(t, err)
	require.Equal(t, "media_limit", reason)
	complete(media)
	var count int
	require.NoError(t, db.QueryRow("SELECT count FROM qqc_daily_usage WHERE app_id=? AND kind='ambient_media'", c.AppID).Scan(&count))
	require.Equal(t, 1, count)
	// A rotated signed URL with the same file identity is rejected even after
	// another media item was received, using the persistent five-minute key.
	route("repeat-media-one", "", true)
	var repeats int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM qqc_jobs WHERE app_id=?", c.AppID).Scan(&repeats))
	require.Equal(t, 4, repeats)
	stats, err := s.autonomousStats(ctx, c)
	require.NoError(t, err)
	found := false
	for _, stat := range stats {
		if stat.Reason == "media_recognitions" {
			found = true
			require.EqualValues(t, 1, stat.Count)
		}
	}
	require.True(t, found)
}
