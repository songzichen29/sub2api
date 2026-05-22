package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type announcementRepoStub struct {
	item *Announcement
}

func (s *announcementRepoStub) Create(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
}

func (s *announcementRepoStub) GetByID(_ context.Context, _ int64) (*Announcement, error) {
	if s.item == nil {
		return nil, ErrAnnouncementNotFound
	}
	return s.item, nil
}

func (s *announcementRepoStub) Update(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
}

func (*announcementRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (*announcementRepoStub) List(context.Context, pagination.PaginationParams, AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (*announcementRepoStub) ListActive(context.Context, time.Time) ([]Announcement, error) {
	return nil, nil
}

type announcementReadStatusUserRepoStub struct {
	UserRepository
	users []User
}

func (s *announcementReadStatusUserRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return s.users, &pagination.PaginationResult{
		Total:    int64(len(s.users)),
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    1,
	}, nil
}

type announcementReadStatusReadRepoStub struct {
	AnnouncementReadRepository
	readMap map[int64]time.Time
}

func (s *announcementReadStatusReadRepoStub) GetReadMapByUsers(_ context.Context, _ int64, _ []int64) (map[int64]time.Time, error) {
	return s.readMap, nil
}

type announcementReadStatusUserSubRepoStub struct {
	UserSubscriptionRepository
}

func (*announcementReadStatusUserSubRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("ListUserReadStatus must use subscriptions already loaded by UserRepository.ListWithFilters")
}

func TestAnnouncementServiceCreateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{}
	svc := NewAnnouncementService(repo, nil, nil, nil)
	now := time.Unix(1776790020, 0)

	_, err := svc.Create(context.Background(), &CreateAnnouncementInput{
		Title:      "公告",
		Content:    "内容",
		Status:     AnnouncementStatusActive,
		NotifyMode: AnnouncementNotifyModePopup,
		StartsAt:   &now,
		EndsAt:     &now,
	})
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
}

func TestAnnouncementServiceListUserReadStatusUsesPreloadedSubscriptions(t *testing.T) {
	now := time.Now()
	readAt := now.Add(-time.Hour)
	repo := &announcementRepoStub{
		item: &Announcement{
			ID:      7,
			Title:   "公告",
			Content: "内容",
			Status:  AnnouncementStatusActive,
			Targeting: AnnouncementTargeting{
				AnyOf: []AnnouncementConditionGroup{
					{
						AllOf: []AnnouncementCondition{
							{
								Type:     AnnouncementConditionTypeSubscription,
								Operator: AnnouncementOperatorIn,
								GroupIDs: []int64{10},
							},
						},
					},
				},
			},
		},
	}
	userRepo := &announcementReadStatusUserRepoStub{
		users: []User{
			{
				ID:       1,
				Email:    "eligible@example.com",
				Username: "eligible",
				Subscriptions: []UserSubscription{
					{
						GroupID:   10,
						Status:    SubscriptionStatusActive,
						ExpiresAt: now.Add(time.Hour),
					},
				},
			},
			{
				ID:       2,
				Email:    "ineligible@example.com",
				Username: "ineligible",
			},
		},
	}
	readRepo := &announcementReadStatusReadRepoStub{
		readMap: map[int64]time.Time{1: readAt},
	}
	svc := NewAnnouncementService(repo, readRepo, userRepo, &announcementReadStatusUserSubRepoStub{})

	got, page, err := svc.ListUserReadStatus(context.Background(), 7, pagination.PaginationParams{
		Page:      1,
		PageSize:  100,
		SortBy:    "email",
		SortOrder: "asc",
	}, "")

	require.NoError(t, err)
	require.NotNil(t, page)
	require.Len(t, got, 2)
	require.True(t, got[0].Eligible)
	require.NotNil(t, got[0].ReadAt)
	require.True(t, got[0].ReadAt.Equal(readAt))
	require.False(t, got[1].Eligible)
	require.Nil(t, got[1].ReadAt)
}

func TestAnnouncementServiceUpdateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{
		item: &Announcement{
			ID:         1,
			Title:      "公告",
			Content:    "内容",
			Status:     AnnouncementStatusActive,
			NotifyMode: AnnouncementNotifyModePopup,
		},
	}
	svc := NewAnnouncementService(repo, nil, nil, nil)
	now := time.Unix(1776790020, 0)
	startsAt := &now
	endsAt := &now

	_, err := svc.Update(context.Background(), 1, &UpdateAnnouncementInput{
		StartsAt: &startsAt,
		EndsAt:   &endsAt,
	})
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
}
