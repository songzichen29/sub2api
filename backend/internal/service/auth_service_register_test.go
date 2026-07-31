//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type settingRepoStub struct {
	values map[string]string
	err    error
}

func (s *settingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *settingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			result[key] = v
		}
	}
	return result, nil
}

func (s *settingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

type emailCacheStub struct {
	data *VerificationCodeData
	err  error
}

type defaultSubscriptionAssignerStub struct {
	calls []AssignSubscriptionInput
	err   error
}

type refreshTokenCacheStub struct{}

type redeemRepoRegisterStub struct {
	useErr   error
	usedID   int64
	usedUser int64
}

func (s *redeemRepoRegisterStub) Create(context.Context, *RedeemCode) error {
	panic("unexpected Create call")
}
func (s *redeemRepoRegisterStub) CreateBatch(context.Context, []RedeemCode) error {
	panic("unexpected CreateBatch call")
}
func (s *redeemRepoRegisterStub) GetByID(context.Context, int64) (*RedeemCode, error) {
	panic("unexpected GetByID call")
}
func (s *redeemRepoRegisterStub) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	switch code {
	case "INVITE-OK":
		return &RedeemCode{ID: 41, Code: code, Type: RedeemTypeInvitation, Status: StatusUnused}, nil
	case "INVITE-USED":
		return &RedeemCode{ID: 42, Code: code, Type: RedeemTypeInvitation, Status: StatusUsed}, nil
	default:
		return nil, ErrRedeemCodeNotFound
	}
}
func (s *redeemRepoRegisterStub) Update(context.Context, *RedeemCode) error {
	panic("unexpected Update call")
}
func (s *redeemRepoRegisterStub) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
}
func (s *redeemRepoRegisterStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (s *redeemRepoRegisterStub) Use(_ context.Context, id, userID int64) error {
	s.usedID = id
	s.usedUser = userID
	return s.useErr
}
func (s *redeemRepoRegisterStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (s *redeemRepoRegisterStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (s *redeemRepoRegisterStub) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}
func (s *redeemRepoRegisterStub) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}
func (s *redeemRepoRegisterStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

type affiliateRepoRegisterStub struct {
	summaries map[int64]*AffiliateSummary
	codes     map[string]int64
	bindErr   error
	bound     map[int64]int64
}

type registerUserRepoWithEnt struct {
	client *dbent.Client
}

func (r *registerUserRepoWithEnt) entClient(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return r.client
}

func (r *registerUserRepoWithEnt) Create(ctx context.Context, user *User) error {
	signupSource := strings.TrimSpace(strings.ToLower(user.SignupSource))
	if signupSource == "" {
		signupSource = "email"
	}
	created, err := r.entClient(ctx).User.Create().
		SetEmail(user.Email).
		SetUsername(user.Username).
		SetNotes(user.Notes).
		SetPasswordHash(user.PasswordHash).
		SetRole(user.Role).
		SetBalance(user.Balance).
		SetConcurrency(user.Concurrency).
		SetStatus(user.Status).
		SetSignupSource(signupSource).
		SetNillableLastLoginAt(user.LastLoginAt).
		SetNillableLastActiveAt(user.LastActiveAt).
		SetRpmLimit(user.RPMLimit).
		Save(ctx)
	if err != nil {
		return err
	}
	user.ID = created.ID
	return nil
}

func (r *registerUserRepoWithEnt) CreateWithEmailAliasGuard(ctx context.Context, user *User) error {
	return r.Create(ctx, user)
}

func (r *registerUserRepoWithEnt) GetByID(ctx context.Context, id int64) (*User, error) {
	entity, err := r.entClient(ctx).User.Get(ctx, id)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &User{
		ID:           entity.ID,
		Email:        entity.Email,
		Username:     entity.Username,
		PasswordHash: entity.PasswordHash,
		Role:         entity.Role,
		Balance:      entity.Balance,
		Concurrency:  entity.Concurrency,
		Status:       entity.Status,
		SignupSource: entity.SignupSource,
	}, nil
}

func (r *registerUserRepoWithEnt) GetByEmail(ctx context.Context, email string) (*User, error) {
	entity, err := r.entClient(ctx).User.Query().Where(dbuser.EmailEQ(email)).Only(ctx)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &User{
		ID:           entity.ID,
		Email:        entity.Email,
		Username:     entity.Username,
		PasswordHash: entity.PasswordHash,
		Role:         entity.Role,
		Balance:      entity.Balance,
		Concurrency:  entity.Concurrency,
		Status:       entity.Status,
		SignupSource: entity.SignupSource,
	}, nil
}

func (r *registerUserRepoWithEnt) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.entClient(ctx).User.Query().Where(dbuser.EmailEQ(email)).Exist(ctx)
}

func (r *registerUserRepoWithEnt) ExistsByEmailAlias(ctx context.Context, email string) (bool, error) {
	identity := NormalizeEmailForAliasDedup(email)
	users, err := r.entClient(ctx).User.Query().Select(dbuser.FieldEmail).All(ctx)
	if err != nil {
		return false, err
	}
	for _, user := range users {
		if NormalizeEmailForAliasDedup(user.Email) == identity {
			return true, nil
		}
	}
	return false, nil
}

func (r *registerUserRepoWithEnt) GetFirstAdmin(context.Context) (*User, error) {
	panic("unexpected GetFirstAdmin call")
}
func (r *registerUserRepoWithEnt) Update(context.Context, *User, UserUpdateFields) error {
	panic("unexpected Update call")
}
func (r *registerUserRepoWithEnt) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (r *registerUserRepoWithEnt) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected GetUserAvatar call")
}
func (r *registerUserRepoWithEnt) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
}
func (r *registerUserRepoWithEnt) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected DeleteUserAvatar call")
}
func (r *registerUserRepoWithEnt) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (r *registerUserRepoWithEnt) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (r *registerUserRepoWithEnt) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs call")
}
func (r *registerUserRepoWithEnt) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID call")
}
func (r *registerUserRepoWithEnt) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateUserLastActiveAt call")
}
func (r *registerUserRepoWithEnt) UpdateBalance(context.Context, int64, float64) error {
	panic("unexpected UpdateBalance call")
}
func (r *registerUserRepoWithEnt) DeductBalance(context.Context, int64, float64) error {
	panic("unexpected DeductBalance call")
}
func (r *registerUserRepoWithEnt) AdjustBalance(context.Context, int64, float64) (BalanceChange, error) {
	panic("unexpected AdjustBalance call")
}
func (r *registerUserRepoWithEnt) SetBalance(context.Context, int64, float64) (BalanceChange, error) {
	panic("unexpected SetBalance call")
}
func (r *registerUserRepoWithEnt) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected UpdateConcurrency call")
}
func (r *registerUserRepoWithEnt) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected BatchSetConcurrency call")
}
func (r *registerUserRepoWithEnt) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected BatchAddConcurrency call")
}
func (r *registerUserRepoWithEnt) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	panic("unexpected BatchUpdateLimits call")
}
func (r *registerUserRepoWithEnt) GetByIDIncludeDeleted(context.Context, int64) (*User, error) {
	panic("unexpected GetByIDIncludeDeleted call")
}
func (r *registerUserRepoWithEnt) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
}
func (r *registerUserRepoWithEnt) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
}
func (r *registerUserRepoWithEnt) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
}
func (r *registerUserRepoWithEnt) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities call")
}
func (r *registerUserRepoWithEnt) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider call")
}
func (r *registerUserRepoWithEnt) UpdateTotpSecret(context.Context, int64, *string) error {
	panic("unexpected UpdateTotpSecret call")
}
func (r *registerUserRepoWithEnt) EnableTotp(context.Context, int64) error {
	panic("unexpected EnableTotp call")
}
func (r *registerUserRepoWithEnt) DisableTotp(context.Context, int64) error {
	panic("unexpected DisableTotp call")
}

func newRegisterAuthServiceWithEnt(
	t *testing.T,
	settings map[string]string,
	redeemRepo RedeemCodeRepository,
	affiliateRepo AffiliateRepository,
) (*AuthService, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:auth_service_register?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config.Config{
		JWT:     config.JWTConfig{Secret: "test-secret", ExpireHour: 1},
		Default: config.DefaultConfig{UserBalance: 3.5, UserConcurrency: 2},
	}
	settingSvc := NewSettingService(&settingRepoStub{values: settings}, cfg)
	affiliateSvc := NewAffiliateService(affiliateRepo, settingSvc, nil, nil)
	userRepo := &registerUserRepoWithEnt{client: client}
	svc := NewAuthService(client, userRepo, redeemRepo, &refreshTokenCacheStub{}, cfg, settingSvc, nil, nil, nil, nil, nil, affiliateSvc, nil)
	return svc, client
}

func (s *affiliateRepoRegisterStub) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	if s.summaries == nil {
		s.summaries = map[int64]*AffiliateSummary{}
	}
	if summary, ok := s.summaries[userID]; ok {
		cp := *summary
		return &cp, nil
	}
	summary := &AffiliateSummary{UserID: userID, AffCode: "AUTO-CODE", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.summaries[userID] = summary
	cp := *summary
	return &cp, nil
}
func (s *affiliateRepoRegisterStub) GetAffiliateByCode(_ context.Context, code string) (*AffiliateSummary, error) {
	if s.codes == nil {
		return nil, ErrAffiliateProfileNotFound
	}
	uid, ok := s.codes[code]
	if !ok {
		return nil, ErrAffiliateProfileNotFound
	}
	return &AffiliateSummary{UserID: uid, AffCode: code, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}
func (s *affiliateRepoRegisterStub) BindInviter(_ context.Context, userID, inviterID int64) (bool, error) {
	if s.bindErr != nil {
		return false, s.bindErr
	}
	if s.bound == nil {
		s.bound = map[int64]int64{}
	}
	if _, exists := s.bound[userID]; exists {
		return false, nil
	}
	s.bound[userID] = inviterID
	if summary, ok := s.summaries[userID]; ok {
		id := inviterID
		summary.InviterID = &id
	}
	return true, nil
}
func (s *affiliateRepoRegisterStub) AccrueQuota(context.Context, int64, int64, float64, int, *int64) (bool, error) {
	panic("unexpected AccrueQuota call")
}
func (s *affiliateRepoRegisterStub) GetAccruedRebateFromInvitee(context.Context, int64, int64) (float64, error) {
	panic("unexpected GetAccruedRebateFromInvitee call")
}
func (s *affiliateRepoRegisterStub) ThawFrozenQuota(context.Context, int64) (float64, error) {
	panic("unexpected ThawFrozenQuota call")
}
func (s *affiliateRepoRegisterStub) TransferQuotaToBalance(context.Context, int64) (float64, float64, error) {
	panic("unexpected TransferQuotaToBalance call")
}
func (s *affiliateRepoRegisterStub) ListInvitees(context.Context, int64, int) ([]AffiliateInvitee, error) {
	panic("unexpected ListInvitees call")
}
func (s *affiliateRepoRegisterStub) UpdateUserAffCode(context.Context, int64, string) error {
	panic("unexpected UpdateUserAffCode call")
}
func (s *affiliateRepoRegisterStub) ResetUserAffCode(context.Context, int64) (string, error) {
	panic("unexpected ResetUserAffCode call")
}
func (s *affiliateRepoRegisterStub) SetUserRebateRates(context.Context, int64, bool, *float64, bool, *float64, bool, *float64) error {
	panic("unexpected SetUserRebateRates call")
}
func (s *affiliateRepoRegisterStub) BatchSetUserRebateRates(context.Context, []int64, bool, *float64, bool, *float64, bool, *float64) error {
	panic("unexpected BatchSetUserRebateRates call")
}
func (s *affiliateRepoRegisterStub) ListUsersWithCustomSettings(context.Context, AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	panic("unexpected ListUsersWithCustomSettings call")
}
func (s *affiliateRepoRegisterStub) ListAffiliateInviteRecords(context.Context, AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	panic("unexpected ListAffiliateInviteRecords call")
}
func (s *affiliateRepoRegisterStub) ListAffiliateRebateRecords(context.Context, AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	panic("unexpected ListAffiliateRebateRecords call")
}
func (s *affiliateRepoRegisterStub) ListAffiliateTransferRecords(context.Context, AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	panic("unexpected ListAffiliateTransferRecords call")
}
func (s *affiliateRepoRegisterStub) GetAffiliateUserOverview(context.Context, int64) (*AffiliateUserOverview, error) {
	panic("unexpected GetAffiliateUserOverview call")
}

func (s *defaultSubscriptionAssignerStub) AssignOrExtendSubscription(_ context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	if input != nil {
		s.calls = append(s.calls, *input)
	}
	if s.err != nil {
		return nil, false, s.err
	}
	return &UserSubscription{UserID: input.UserID, GroupID: input.GroupID}, false, nil
}

func (s *refreshTokenCacheStub) StoreRefreshToken(context.Context, string, *RefreshTokenData, time.Duration) error {
	return nil
}

func (s *refreshTokenCacheStub) GetRefreshToken(context.Context, string) (*RefreshTokenData, error) {
	return nil, ErrRefreshTokenNotFound
}

func (s *refreshTokenCacheStub) DeleteRefreshToken(context.Context, string) error {
	return nil
}

func (s *refreshTokenCacheStub) DeleteUserRefreshTokens(context.Context, int64) error {
	return nil
}

func (s *refreshTokenCacheStub) DeleteTokenFamily(context.Context, string) error {
	return nil
}

func (s *refreshTokenCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *refreshTokenCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *refreshTokenCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *refreshTokenCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
}

func (s *refreshTokenCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s *emailCacheStub) GetVerificationCode(ctx context.Context, email string) (*VerificationCodeData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func (s *emailCacheStub) SetVerificationCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error {
	return nil
}

func (s *emailCacheStub) DeleteVerificationCode(ctx context.Context, email string) error {
	return nil
}

func (s *emailCacheStub) GetNotifyVerifyCode(ctx context.Context, email string) (*VerificationCodeData, error) {
	return nil, nil
}

func (s *emailCacheStub) SetNotifyVerifyCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error {
	return nil
}

func (s *emailCacheStub) DeleteNotifyVerifyCode(ctx context.Context, email string) error {
	return nil
}

func (s *emailCacheStub) GetPasswordResetToken(ctx context.Context, email string) (*PasswordResetTokenData, error) {
	return nil, nil
}

func (s *emailCacheStub) SetPasswordResetToken(ctx context.Context, email string, data *PasswordResetTokenData, ttl time.Duration) error {
	return nil
}

func (s *emailCacheStub) DeletePasswordResetToken(ctx context.Context, email string) error {
	return nil
}

func (s *emailCacheStub) IsPasswordResetEmailInCooldown(ctx context.Context, email string) bool {
	return false
}

func (s *emailCacheStub) SetPasswordResetEmailCooldown(ctx context.Context, email string, ttl time.Duration) error {
	return nil
}

func (s *emailCacheStub) GetNotifyCodeUserRate(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
}

func (s *emailCacheStub) IncrNotifyCodeUserRate(ctx context.Context, userID int64, window time.Duration) (int64, error) {
	return 0, nil
}

func newAuthService(repo *userRepoStub, settings map[string]string, emailCache EmailCache, quotaRepos ...UserPlatformQuotaRepository) *AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 1,
		},
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
		},
	}

	var settingService *SettingService
	if settings != nil {
		settingService = NewSettingService(&settingRepoStub{values: settings}, cfg)
	}

	var emailService *EmailService
	if emailCache != nil {
		emailService = NewEmailService(&settingRepoStub{values: settings}, emailCache)
	}

	var quotaRepo UserPlatformQuotaRepository
	if len(quotaRepos) > 0 {
		quotaRepo = quotaRepos[0]
	}

	return NewAuthService(
		nil, // entClient
		repo,
		nil, // redeemRepo
		nil, // refreshTokenCache
		cfg,
		settingService,
		emailService,
		nil,
		nil,
		nil,       // promoService
		nil,       // defaultSubAssigner
		nil,       // affiliateService
		quotaRepo, // userPlatformQuotaRepo
	)
}

func TestAuthService_Register_Disabled(t *testing.T) {
	repo := &userRepoStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "false",
	}, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrRegDisabled)
}

func TestAuthService_Register_DisabledByDefault(t *testing.T) {
	// 当 settings 为 nil（设置项不存在）时，注册应该默认关闭
	repo := &userRepoStub{}
	service := newAuthService(repo, nil, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrRegDisabled)
}

func TestAuthService_Register_EmailVerifyEnabledButServiceNotConfigured(t *testing.T) {
	repo := &userRepoStub{}
	// 邮件验证开启但 emailCache 为 nil（emailService 未配置）
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
	}, nil)

	// 应返回服务不可用错误，而不是允许绕过验证
	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "any-code", "", "", "")
	require.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestAuthService_Register_EmailVerifyRequired(t *testing.T) {
	repo := &userRepoStub{}
	cache := &emailCacheStub{} // 配置 emailService
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
	}, cache)

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "", "", "", "")
	require.ErrorIs(t, err, ErrEmailVerifyRequired)
}

func TestAuthService_Register_EmailVerifyInvalid(t *testing.T) {
	repo := &userRepoStub{}
	cache := &emailCacheStub{
		data: &VerificationCodeData{Code: "expected", Attempts: 0},
	}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "true",
	}, cache)

	_, _, err := service.RegisterWithVerification(context.Background(), "user@test.com", "password", "wrong", "", "", "")
	require.ErrorIs(t, err, ErrInvalidVerifyCode)
	require.ErrorContains(t, err, "verify code")
}

func TestAuthService_Register_EmailExists(t *testing.T) {
	repo := &userRepoStub{exists: true}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
}

func TestAuthService_Register_AliasDuplicateRejected(t *testing.T) {
	repo := &userRepoStub{aliasExists: true}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil, nil)

	_, _, err := service.Register(context.Background(), "some.one+bulk294@gmail.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
	require.Empty(t, repo.created)
}

func TestAuthService_Register_UsesAliasGuardedCreate(t *testing.T) {
	// 注册必须走带别名兜底的创建路径：服务层前置查重与写入之间存在竞态窗口。
	repo := &userRepoStub{nextID: 91}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil, nil)

	_, user, err := service.Register(context.Background(), "newuser@gmail.com", "password")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, 1, repo.guardedCreates)
}

func TestAuthService_Register_CheckEmailError(t *testing.T) {
	repo := &userRepoStub{existsErr: errors.New("db down")}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestAuthService_Register_ReservedEmail(t *testing.T) {
	repo := &userRepoStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil)

	_, _, err := service.Register(context.Background(), "linuxdo-123@linuxdo-connect.invalid", "password")
	require.ErrorIs(t, err, ErrEmailReserved)
}

func TestAuthService_Register_EmailSuffixNotAllowed(t *testing.T) {
	repo := &userRepoStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com","@company.com"]`,
	}, nil)

	_, _, err := service.Register(context.Background(), "user@other.com", "password")
	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
	appErr := infraerrors.FromError(err)
	require.Contains(t, appErr.Message, "@example.com")
	require.Contains(t, appErr.Message, "@company.com")
	require.Equal(t, "EMAIL_SUFFIX_NOT_ALLOWED", appErr.Reason)
	require.Equal(t, "2", appErr.Metadata["allowed_suffix_count"])
	require.Equal(t, "@example.com,@company.com", appErr.Metadata["allowed_suffixes"])
}

func TestAuthService_Register_EmailSuffixAllowed(t *testing.T) {
	repo := &userRepoStub{nextID: 8}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["example.com"]`,
	}, nil)

	_, user, err := service.Register(context.Background(), "user@example.com", "password")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, int64(8), user.ID)
}

func TestAuthService_RegisterWithVerification_InvitationAndAffiliateAreAtomic(t *testing.T) {
	redeemRepo := &redeemRepoRegisterStub{useErr: ErrRedeemCodeUsed}
	affiliateRepo := &affiliateRepoRegisterStub{
		codes: map[string]int64{
			"AFF123": 99,
		},
	}
	svc, client := newRegisterAuthServiceWithEnt(t, map[string]string{
		SettingKeyRegistrationEnabled:   "true",
		SettingKeyInvitationCodeEnabled: "true",
		SettingKeyAffiliateEnabled:      "true",
	}, redeemRepo, affiliateRepo)

	_, _, err := svc.RegisterWithVerification(context.Background(), "atomic@example.com", "password", "", "", "INVITE-OK", "AFF123")
	require.ErrorIs(t, err, ErrInvitationCodeInvalid)

	count, queryErr := client.User.Query().Count(context.Background())
	require.NoError(t, queryErr)
	require.Equal(t, 0, count, "邀请码核销失败时，用户创建必须整体回滚")
}

func TestAuthService_RegisterWithVerification_AffiliateBindFailureRollsBackUser(t *testing.T) {
	redeemRepo := &redeemRepoRegisterStub{}
	affiliateRepo := &affiliateRepoRegisterStub{
		codes: map[string]int64{
			"BAD-AFF": 77,
		},
		bindErr: ErrAffiliateCodeInvalid,
	}
	svc, client := newRegisterAuthServiceWithEnt(t, map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyAffiliateEnabled:    "true",
	}, redeemRepo, affiliateRepo)

	_, _, err := svc.RegisterWithVerification(context.Background(), "rollback-aff@example.com", "password", "", "", "", "BAD-AFF")
	require.ErrorIs(t, err, ErrAffiliateCodeInvalid)

	count, queryErr := client.User.Query().Count(context.Background())
	require.NoError(t, queryErr)
	require.Equal(t, 0, count, "返利绑定失败时，用户创建必须回滚")
}

func TestAuthService_SendVerifyCode_EmailSuffixNotAllowed(t *testing.T) {
	repo := &userRepoStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com","@company.com"]`,
	}, nil)

	err := service.SendVerifyCode(context.Background(), "user@other.com")
	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
	appErr := infraerrors.FromError(err)
	require.Contains(t, appErr.Message, "@example.com")
	require.Contains(t, appErr.Message, "@company.com")
	require.Equal(t, "2", appErr.Metadata["allowed_suffix_count"])
}

func TestAuthService_Register_CreateError(t *testing.T) {
	repo := &userRepoStub{createErr: errors.New("create failed")}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestAuthService_Register_CreateEmailExistsRace(t *testing.T) {
	// 模拟竞态条件：ExistsByEmail 返回 false，但 Create 时因唯一约束失败
	repo := &userRepoStub{createErr: ErrEmailExists}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, nil)

	_, _, err := service.Register(context.Background(), "user@test.com", "password")
	require.ErrorIs(t, err, ErrEmailExists)
}

func TestAuthService_Register_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 5}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "false",
	}, nil)

	token, user, err := service.Register(context.Background(), "user@test.com", "password")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotNil(t, user)
	require.Equal(t, int64(5), user.ID)
	require.Equal(t, "user@test.com", user.Email)
	require.Equal(t, RoleUser, user.Role)
	require.Equal(t, StatusActive, user.Status)
	require.Equal(t, 3.5, user.Balance)
	require.Equal(t, 2, user.Concurrency)
	require.Len(t, repo.created, 1)
	require.True(t, user.CheckPassword("password"))
}

func TestAuthService_ValidateToken_ExpiredReturnsClaimsWithError(t *testing.T) {
	repo := &userRepoStub{}
	service := newAuthService(repo, nil, nil)

	// 创建用户并生成 token
	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
	}
	token, err := service.GenerateToken(context.Background(), user)
	require.NoError(t, err)

	// 验证有效 token
	claims, err := service.ValidateToken(token)
	require.NoError(t, err)
	require.NotNil(t, claims)
	require.Equal(t, int64(1), claims.UserID)

	// 模拟过期 token（通过创建一个过期很久的 token）
	service.cfg.JWT.ExpireHour = -1 // 设置为负数使 token 立即过期
	expiredToken, err := service.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	service.cfg.JWT.ExpireHour = 1 // 恢复

	// 验证过期 token 应返回 claims 和 ErrTokenExpired
	claims, err = service.ValidateToken(expiredToken)
	require.ErrorIs(t, err, ErrTokenExpired)
	require.NotNil(t, claims, "claims should not be nil when token is expired")
	require.Equal(t, int64(1), claims.UserID)
	require.Equal(t, "test@test.com", claims.Email)
}

func TestAuthService_RefreshToken_ExpiredTokenNoPanic(t *testing.T) {
	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
	}
	repo := &userRepoStub{user: user}
	service := newAuthService(repo, nil, nil)

	// 创建过期 token
	service.cfg.JWT.ExpireHour = -1
	expiredToken, err := service.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	service.cfg.JWT.ExpireHour = 1

	// RefreshToken 使用过期 token 不应 panic
	require.NotPanics(t, func() {
		newToken, err := service.RefreshToken(context.Background(), expiredToken)
		require.NoError(t, err)
		require.NotEmpty(t, newToken)
	})
}

func TestAuthService_GetAccessTokenExpiresIn_FallbackToExpireHour(t *testing.T) {
	service := newAuthService(&userRepoStub{}, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 0

	require.Equal(t, 24*3600, service.GetAccessTokenExpiresIn())
}

func TestAuthService_GetAccessTokenExpiresIn_MinutesHasPriority(t *testing.T) {
	service := newAuthService(&userRepoStub{}, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 90

	require.Equal(t, 90*60, service.GetAccessTokenExpiresIn())
}

func TestAuthService_GenerateToken_UsesExpireHourWhenMinutesZero(t *testing.T) {
	service := newAuthService(&userRepoStub{}, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 0

	user := &User{
		ID:           1,
		Email:        "test@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
	}

	token, err := service.GenerateToken(context.Background(), user)
	require.NoError(t, err)

	claims, err := service.ValidateToken(token)
	require.NoError(t, err)
	require.NotNil(t, claims)
	require.NotNil(t, claims.IssuedAt)
	require.NotNil(t, claims.ExpiresAt)

	require.WithinDuration(t, claims.IssuedAt.Time.Add(24*time.Hour), claims.ExpiresAt.Time, 2*time.Second)
}

func TestAuthService_GenerateToken_UsesMinutesWhenConfigured(t *testing.T) {
	service := newAuthService(&userRepoStub{}, nil, nil)
	service.cfg.JWT.ExpireHour = 24
	service.cfg.JWT.AccessTokenExpireMinutes = 90

	user := &User{
		ID:           2,
		Email:        "test2@test.com",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 1,
	}

	token, err := service.GenerateToken(context.Background(), user)
	require.NoError(t, err)

	claims, err := service.ValidateToken(token)
	require.NoError(t, err)
	require.NotNil(t, claims)
	require.NotNil(t, claims.IssuedAt)
	require.NotNil(t, claims.ExpiresAt)

	require.WithinDuration(t, claims.IssuedAt.Time.Add(90*time.Minute), claims.ExpiresAt.Time, 2*time.Second)
}

func TestAuthService_Register_AssignsDefaultSubscriptions(t *testing.T) {
	repo := &userRepoStub{nextID: 42}
	assigner := &defaultSubscriptionAssignerStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":11,"validity_days":30},{"group_id":12,"validity_days":7}]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "false",
	}, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "default-sub@test.com", "password")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Len(t, assigner.calls, 2)
	require.Equal(t, int64(42), assigner.calls[0].UserID)
	require.Equal(t, int64(11), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
	require.Equal(t, int64(12), assigner.calls[1].GroupID)
	require.Equal(t, 7, assigner.calls[1].ValidityDays)
}

func TestAuthService_Register_UsesEmailAuthSourceDefaultsWhenGrantEnabled(t *testing.T) {
	repo := &userRepoStub{nextID: 52}
	assigner := &defaultSubscriptionAssignerStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":91,"validity_days":3}]`,
		SettingKeyAuthSourceDefaultEmailBalance:       "12.5",
		SettingKeyAuthSourceDefaultEmailConcurrency:   "7",
		SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":11,"validity_days":30}]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "true",
	}, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "email-defaults@test.com", "password")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, 12.5, user.Balance)
	require.Equal(t, 7, user.Concurrency)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(11), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
}

func TestAuthService_Register_GrantOnSignupFalseFallsBackToGlobalDefaults(t *testing.T) {
	repo := &userRepoStub{nextID: 53}
	assigner := &defaultSubscriptionAssignerStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":31,"validity_days":5}]`,
		SettingKeyAuthSourceDefaultEmailBalance:       "99",
		SettingKeyAuthSourceDefaultEmailConcurrency:   "88",
		SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":32,"validity_days":9}]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "false",
	}, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "email-global@test.com", "password")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, 3.5, user.Balance)
	require.Equal(t, 2, user.Concurrency)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(31), assigner.calls[0].GroupID)
	require.Equal(t, 5, assigner.calls[0].ValidityDays)
}

func TestAuthService_Register_GrantOnSignupMergesSourceOverridesWithGlobalDefaults(t *testing.T) {
	repo := &userRepoStub{nextID: 54}
	assigner := &defaultSubscriptionAssignerStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyDefaultSubscriptions:                `[{"group_id":31,"validity_days":5}]`,
		SettingKeyAuthSourceDefaultEmailBalance:       "9.5",
		SettingKeyAuthSourceDefaultEmailConcurrency:   "5",
		SettingKeyAuthSourceDefaultEmailSubscriptions: `[]`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup: "true",
	}, nil)
	service.defaultSubAssigner = assigner

	_, user, err := service.Register(context.Background(), "email-merged@test.com", "password")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, 9.5, user.Balance)
	require.Equal(t, 5, user.Concurrency)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(31), assigner.calls[0].GroupID)
	require.Equal(t, 5, assigner.calls[0].ValidityDays)
}

func TestAuthService_LoginOrRegisterOAuthWithTokenPair_UsesLinuxDoAuthSourceDefaultsOnSignup(t *testing.T) {
	repo := &userRepoStub{nextID: 61}
	assigner := &defaultSubscriptionAssignerStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                   "true",
		SettingKeyDefaultSubscriptions:                  `[{"group_id":81,"validity_days":1}]`,
		SettingKeyAuthSourceDefaultLinuxDoBalance:       "21.75",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:   "9",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions: `[{"group_id":22,"validity_days":14}]`,
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup: "true",
	}, nil)
	service.defaultSubAssigner = assigner
	service.refreshTokenCache = &refreshTokenCacheStub{}

	tokenPair, user, err := service.LoginOrRegisterOAuthWithTokenPair(context.Background(), "linuxdo-123@linuxdo-connect.invalid", "linuxdo_user", "", "", "")
	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Equal(t, int64(61), user.ID)
	require.Equal(t, 21.75, user.Balance)
	require.Equal(t, 9, user.Concurrency)
	require.Len(t, repo.created, 1)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(22), assigner.calls[0].GroupID)
	require.Equal(t, 14, assigner.calls[0].ValidityDays)
}

func TestAuthService_LoginOrRegisterOAuthWithTokenPair_ExistingUserDoesNotGrantAgain(t *testing.T) {
	existing := &User{
		ID:           88,
		Email:        "linuxdo-123@linuxdo-connect.invalid",
		Username:     "existing-linuxdo",
		Role:         RoleUser,
		Status:       StatusActive,
		Balance:      4,
		Concurrency:  1,
		TokenVersion: 2,
	}
	repo := &userRepoStub{user: existing}
	assigner := &defaultSubscriptionAssignerStub{}
	service := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                   "true",
		SettingKeyAuthSourceDefaultLinuxDoBalance:       "21.75",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:   "9",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions: `[{"group_id":22,"validity_days":14}]`,
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup: "true",
	}, nil)
	service.defaultSubAssigner = assigner
	service.refreshTokenCache = &refreshTokenCacheStub{}

	tokenPair, user, err := service.LoginOrRegisterOAuthWithTokenPair(context.Background(), existing.Email, "linuxdo_user", "", "", "")
	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.Equal(t, existing.ID, user.ID)
	require.Equal(t, 4.0, user.Balance)
	require.Equal(t, 1, user.Concurrency)
	require.Empty(t, repo.created)
	require.Empty(t, assigner.calls)
}
