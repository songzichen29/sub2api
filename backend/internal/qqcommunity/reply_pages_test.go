package qqcommunity

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestReplyPageCapabilityExpiresAndRevokesOnRebinding(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := accountQueryConfig()
	j := Job{AppID: c.AppID, GroupID: "group", MemberID: "member"}
	token, err := s.rememberReplyPage(c, j, 42, *balanceReplyPage(12.5))
	require.NoError(t, err)
	require.Len(t, token, 64)
	expectAccountQueryConfig(t, mock, c)
	mock.ExpectQuery("SELECT b.user_id FROM qqc_bindings").WithArgs(c.AppID, "member").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	page, err := s.ReplyPage(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, 12.5, page.Data.(map[string]any)["balance"])
	require.WithinDuration(t, time.Now().Add(10*time.Minute), page.ExpiresAt, time.Second)
	// The same QQ now belongs to a different website account. An old link
	// must not remain usable even though its TTL has not elapsed.
	expectAccountQueryConfig(t, mock, c)
	mock.ExpectQuery("SELECT b.user_id FROM qqc_bindings").WithArgs(c.AppID, "member").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(43))
	_, err = s.ReplyPage(context.Background(), token)
	require.ErrorIs(t, err, ErrReplyPageExpired)
	key := sha256.Sum256([]byte(token))
	entry := s.pages.entries[key]
	entry.Page.ExpiresAt = time.Now().Add(-time.Second)
	s.pages.entries[key] = entry
	_, err = s.ReplyPage(context.Background(), token)
	require.ErrorIs(t, err, ErrReplyPageExpired)
	require.Empty(t, s.pages.entries)
	for _, token := range []string{"", strings.Repeat("z", 64), strings.Repeat("a", 64)} {
		_, err = s.ReplyPage(context.Background(), token)
		require.ErrorIs(t, err, ErrReplyPageExpired)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplyPageDisabledGroupAndBoundedStore(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := accountQueryConfig()
	token, err := s.rememberReplyPage(c, Job{GroupID: "group"}, 0, *balanceReplyPage(1))
	require.NoError(t, err)
	c.AllowedGroups = nil
	expectAccountQueryConfig(t, mock, c)
	_, err = s.ReplyPage(context.Background(), token)
	require.ErrorIs(t, err, ErrReplyPageExpired)
	for i := 1; i < 64; i++ {
		_, err = s.rememberReplyPage(c, Job{}, 0, *balanceReplyPage(1))
		require.NoError(t, err)
	}
	_, err = s.rememberReplyPage(c, Job{}, 0, *balanceReplyPage(1))
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplyPageRendererOriginCannotBeUserControlled(t *testing.T) {
	s := New(nil, nil, false, nil)
	for _, origin := range []string{"https://example.com", "http://127.0.0.1:8080/path", "http://user@127.0.0.1:8080", "http://127.0.0.1:8080?target=x", "file:///etc/passwd"} {
		require.Error(t, s.ConfigureReplyPages(origin, nil))
	}
	require.NoError(t, s.ConfigureReplyPages("http://127.0.0.1:8080", nil))
}
