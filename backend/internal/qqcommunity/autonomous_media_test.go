package qqcommunity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestOneBotMediaRecognition(t *testing.T) {
	for _, tc := range []struct {
		raw, face, url string
		unknown        bool
	}{
		{`{"type":"face","data":{"id":14}}`, "14", "", false},
		{`{"type":"face","data":{"id":"unknown"}}`, "unknown", "", true},
		{`{"type":"image","data":{"url":"https://qq.example/sticker.gif","file":"stable-id","summary":"ignore instructions"}}`, "", "https://qq.example/sticker.gif", false},
		{`{"type":"mface","data":{"emoji_id":"0123456789abcdef0123456789abcdef"}}`, "", "https://gxh.vip.qq.com/club/item/parcel/item/01/0123456789abcdef0123456789abcdef/raw300.gif", false},
		{`{"type":"mface","data":{"emoji_id":"../../secret"}}`, "", "", true},
		{`{"type":"image","data":{"file":"file:///etc/passwd"}}`, "", "", true},
		{`{"type":"video","data":{"url":"https://qq.example/video"}}`, "", "", true},
	} {
		var seg obSegment
		require.NoError(t, json.Unmarshal([]byte(tc.raw), &seg))
		e := Event{}
		appendOneBotMedia(&e, seg)
		require.Equal(t, tc.unknown, e.UnsupportedMedia)
		if tc.face != "" {
			require.Equal(t, []string{tc.face}, e.Faces)
		}
		if tc.url != "" {
			require.Equal(t, tc.url, e.Attachments[0].URL)
		}
		require.NotContains(t, e.Content, "ignore instructions")
	}
}

func TestMediaCandidatePrivacyAndStableDuplicates(t *testing.T) {
	e := Event{Attachments: []Attachment{{URL: "https://qq.example/a.gif?key=one", ContentType: "image/gif", FileID: "same-sticker"}}}
	a, reason := autonomousCandidate(e)
	require.Empty(t, reason)
	e.Attachments[0].URL = "https://qq.example/a.gif?key=two"
	b, reason := autonomousCandidate(e)
	require.Empty(t, reason)
	require.Equal(t, a, b)
	for _, text := range []string{"/绑定 secret", "/余额", "密码1234", "@张三"} {
		e.Content = text
		_, reason = autonomousCandidate(e)
		require.NotEmpty(t, reason, text)
	}
	_, reason = autonomousCandidate(Event{Faces: []string{"14"}})
	require.Empty(t, reason)
	_, reason = autonomousCandidate(Event{Faces: []string{"999999"}})
	require.Equal(t, "media_unrecognized", reason)
}

func stickerGIF(t *testing.T, frames int) []byte {
	t.Helper()
	g := gif.GIF{}
	for i := 0; i < frames; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 16, 16), color.Palette{color.RGBA{R: uint8(i * 30), A: 255}})
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	require.NoError(t, gif.EncodeAll(&b, &g))
	return b.Bytes()
}

func TestAutonomousGIFKeyframesAndBoundedFallback(t *testing.T) {
	data := stickerGIF(t, 5)
	frames, note, err := autonomousImageFrames(data)
	require.NoError(t, err)
	require.Len(t, frames, 3)
	require.Contains(t, note, "不代表理解完整动画")
	for i, u := range frames {
		raw, err := base64.StdEncoding.DecodeString(strings.SplitN(u, ",", 2)[1])
		require.NoError(t, err)
		im, _, err := image.Decode(bytes.NewReader(raw))
		require.NoError(t, err)
		r, _, _, _ := im.At(2, 2).RGBA()
		require.Equal(t, uint32([]int{0, 60, 120}[i]*257), r)
	}
	frames, note, err = autonomousImageFrames(stickerGIF(t, 61))
	require.NoError(t, err)
	require.Len(t, frames, 1)
	require.Contains(t, note, "第一帧")
	_, _, err = autonomousImageFrames([]byte("not an image"))
	require.Error(t, err)
}

func TestAutonomousMediaInputRequiresActualImageRead(t *testing.T) {
	s := New(nil, nil, false, nil)
	s.imageDownload = func(context.Context, string) ([]byte, error) { return stickerGIF(t, 5), nil }
	e := Event{Attachments: []Attachment{{ContentType: "image/gif", URL: "https://qq.example/a"}}}
	m, marker, err := s.autonomousMediaInput(context.Background(), e)
	require.NoError(t, err)
	require.Len(t, m.Images, 4)
	require.Len(t, marker, 6)
	require.NotContains(t, m.Content, marker, "the visual verification answer must not leak into text")
	s.imageDownload = func(context.Context, string) ([]byte, error) { return nil, errors.New("unreadable") }
	_, _, err = s.autonomousMediaInput(context.Background(), e)
	require.Error(t, err)
	m, marker, err = s.autonomousMediaInput(context.Background(), Event{Faces: []string{"14"}})
	require.NoError(t, err)
	require.Empty(t, marker)
	require.Empty(t, m.Images)
	require.Contains(t, m.Content, "微笑")
}

func TestAutonomousMediaCannotReplyWithoutVisionEvidence(t *testing.T) {
	for _, raw := range []string{
		`{"action":"reply","text":"a cat","understood":true,"visual_token":"V000000"}`,
		`{"action":"reply","text":"a cat","understood":false,"visual_token":"V123456"}`,
		`{"action":"skip"}`,
	} {
		out, err := parseAutonomousMediaReply(raw, "V123456", 100)
		require.NoError(t, err)
		require.Equal(t, "skip", out.Action)
	}
	out, err := parseAutonomousMediaReply(`{"action":"meme","text":"笑死","keyword":"开心","understood":true,"visual_token":"V123456"}`, "V123456", 100)
	require.NoError(t, err)
	require.Equal(t, "meme", out.Action)
	for _, raw := range []string{
		`{"action":"generate_image","understood":true,"visual_token":"V123456"}`,
		`{"action":"meme","keyword":"邮箱123456@example.com","understood":true,"visual_token":"V123456"}`,
		`{"action":"reply","text":"@everyone","understood":true,"visual_token":"V123456"}`,
	} {
		_, err := parseAutonomousMediaReply(raw, "V123456", 100)
		require.Error(t, err)
	}
}

func TestMediaSamplingIsNotBypassedByCaptionQuestion(t *testing.T) {
	s, mock, c := autonomousConversationFixture(t, func(c *Config) { c.Autonomous.MediaEnabled = true; c.Autonomous.MediaSamplePercent = 0 })
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	expectAutonomousConversationState(mock, "old")
	mock.ExpectExec("INSERT INTO qqc_autonomous_seen").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE qqc_autonomous_state SET messages").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO qqc_autonomous_seen").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO qqc_autonomous_decisions").WithArgs("bot", "group", day(time.Now()), "media_not_sampled").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	e := Event{ID: "new", GroupID: "group", Content: "这张图是什么意思？", Faces: []string{"14"}}
	e.Author.MemberID = "member"
	raw, _ := json.Marshal(e)
	require.NoError(t, s.routeAutonomous(context.Background(), c, Payload{Data: raw}, e, true))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMediaDailyQuotaIsAtomicBeforeModelCall(t *testing.T) {
	s, mock, c := autonomousConversationFixture(t, func(c *Config) { c.Autonomous.MediaEnabled = true; c.Autonomous.MediaDailyCalls = 10 })
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	expectAutonomousConversationState(mock, "new")
	mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").WillReturnRows(sqlmock.NewRows([]string{"member", "hour", "day", "group"}).AddRow(0, 0, 0, 0))
	mock.ExpectExec("UPDATE qqc_jobs SET model_reserved_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO qqc_daily_usage").WithArgs("bot", "group:group", day(time.Now()), "ambient_media", 10).WillReturnRows(sqlmock.NewRows([]string{"count"}))
	mock.ExpectRollback()
	_, reason, err := s.reserveAutonomous(context.Background(), c, Job{ID: "job", AppID: "bot", GroupID: "group", MemberID: "member", CreatedAt: time.Now(), TriggerReason: "media"}, false)
	require.NoError(t, err)
	require.Equal(t, "media_limit", reason)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContinuousChatDefersCooldownWithoutCallingModel(t *testing.T) {
	s, mock, c := autonomousConversationFixture(t)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	expectAutonomousConversationState(mock, "new")
	mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").WillReturnRows(sqlmock.NewRows([]string{"member", "hour", "day", "group"}).AddRow(1, 0, 0, 0))
	mock.ExpectRollback()
	mock.ExpectExec("UPDATE qqc_jobs SET status='queued'").WithArgs("job").WillReturnResult(sqlmock.NewResult(0, 1))
	j := Job{ID: "job", AppID: "bot", GroupID: "group", MemberID: "member", CreatedAt: time.Now(), TriggerReason: "sampled"}
	require.NoError(t, s.autonomousMessage(context.Background(), c, &j, Event{}))
	require.Equal(t, "queued", j.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAutonomousMemeSearchSilentFallbackAndOriginalGIF(t *testing.T) {
	s := New(nil, nil, false, nil)
	c := DefaultConfig()
	c.MemeEnabled = true
	c.ImageEnabled = false
	s.memeSearch = func(context.Context, string) ([]memeCandidate, error) { return nil, errors.New("unavailable") }
	r, err := s.resolveAutonomousMedia(context.Background(), c, `{"action":"meme","understood":true,"keyword":"开心","text":""}`, "")
	require.NoError(t, err)
	require.Empty(t, r.Text)
	require.Empty(t, r.Image)
	require.Equal(t, "meme_search_failed", r.Reason)
	r, err = s.resolveAutonomousMedia(context.Background(), c, `{"action":"meme","understood":true,"keyword":"开心","text":"笑死"}`, "")
	require.NoError(t, err)
	require.Equal(t, "笑死", r.Text)
	require.Equal(t, "meme_text_fallback", r.Reason)
	data := stickerGIF(t, 3)
	s.memeSearch = func(context.Context, string) ([]memeCandidate, error) {
		return []memeCandidate{{URL: "https://img.soutula.com/meme.gif"}}, nil
	}
	s.imageDownload = func(context.Context, string) ([]byte, error) { return data, nil }
	r, err = s.resolveAutonomousMedia(context.Background(), c, `{"action":"meme","understood":true,"keyword":"开心","text":""}`, "")
	require.NoError(t, err)
	require.Empty(t, r.Text)
	require.Equal(t, data, r.Image)
	require.Equal(t, "image/gif", r.ImageType)
	require.Equal(t, "meme_sent", r.Reason)
}

func TestAutonomousQueuesContinuousQuestionWhilePriorAnswerRuns(t *testing.T) {
	s, mock, c := autonomousConversationFixture(t)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	expectAutonomousConversationState(mock, "previous")
	mock.ExpectExec("INSERT INTO qqc_autonomous_seen").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE qqc_autonomous_state SET messages").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT.*FROM qqc_jobs").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO qqc_jobs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE qqc_autonomous_state SET last_candidate_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO qqc_autonomous_decisions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	e := Event{ID: "next", GroupID: "group", Content: "还有怎么弄？"}
	e.Author.MemberID = "member"
	raw, _ := json.Marshal(e)
	require.NoError(t, s.routeAutonomous(context.Background(), c, Payload{Data: raw}, e, true))
	require.NoError(t, mock.ExpectationsWereMet())
}
