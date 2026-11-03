package qqcommunity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCurrentTimeUsesBeijingDateAcrossUTCMidnight(t *testing.T) {
	m := currentTimeMessage(time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC))
	require.Contains(t, m.Content, "2026-11-03 01:00:00，星期二")
}

func TestExplicitChatRefreshesBindingAndAdminWithoutHistory(t *testing.T) {
	for _, tc := range []struct {
		name, role, state string
		err               error
	}{
		{"bound", "admin", "bound", nil}, {"unbound", "", "unbound", sql.ErrNoRows}, {"unavailable", "", "unknown", errors.New("private-database-error")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			s := New(db, nil, false, nil)
			c := accountQueryConfig()
			c.HistoryMessages = 0
			c.GroupAdmins = []GroupAdmin{{AppID: c.AppID, GroupID: "group", MemberID: "member", Nickname: "老爹"}}
			query := mock.ExpectQuery("SELECT u.role FROM qqc_bindings").WithArgs(c.AppID, "member")
			if tc.err != nil {
				query.WillReturnError(tc.err)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(tc.role))
			}
			messages := s.historyForQuestion(context.Background(), c, Job{AppID: c.AppID, GroupID: "group", MemberID: "member"}, "我是谁")
			text := fmt.Sprint(messages)
			require.Contains(t, text, `"binding":"`+tc.state+`"`)
			require.Contains(t, text, `"approved_nickname":"老爹"`)
			require.Contains(t, text, "后端本轮当前时间")
			require.NotContains(t, text, "private-database-error")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRetiredCommandsAbsentFromHelpAndCannotBecomeCustomCommands(t *testing.T) {
	c := DefaultConfig()
	for _, command := range []string{"邀请奖励", "邀请活动", "我的邀请", "价格"} {
		require.True(t, isRetiredCommand(command))
		_, ok := customCommandReply([]CustomCommand{{Command: "/" + command, Enabled: true, Reply: "old"}}, command, "")
		require.False(t, ok)
		require.NotContains(t, chatPreviewHelp(c), command)
	}
	// Existing account/recharge callbacks cannot award the removed activity.
	require.NoError(t, New(nil, nil, false, nil).SettleInvites(context.Background(), "member"))
}
