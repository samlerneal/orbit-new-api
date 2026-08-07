package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type selectCountingLogger struct {
	selectCount int
}

func (l *selectCountingLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l *selectCountingLogger) Info(context.Context, string, ...interface{})  {}
func (l *selectCountingLogger) Warn(context.Context, string, ...interface{})  {}
func (l *selectCountingLogger) Error(context.Context, string, ...interface{}) {}
func (l *selectCountingLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
		l.selectCount++
	}
}

func setupManageUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Log{}, &model.CasbinRule{}, &model.AuthzRole{},
		&model.BonusBalance{},
	))

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func insertManageUserFixture(t *testing.T, db *gorm.DB, id int, user *model.User) {
	t.Helper()
	user.Id = id
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password, role, status, "group", auth_version, quota, aff_code) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, user.Username, user.Password, user.Role, user.Status, user.Group, user.AuthVersion, user.Quota, user.AffCode).Error)
}

func TestBuildAdminUserListItemsKeepsBonusFieldsOutOfGenericUserJSON(t *testing.T) {
	db := setupManageUserTestDB(t)
	now := time.Now().Unix()
	user := model.User{Username: "admin-list-user", Email: "user@example.com", Quota: 300}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.BonusBalance{
		UserId: user.Id, Status: model.BonusBalanceStatusActive,
		AmountTotal: 100, AmountUsed: 25, ExpiresAt: now + 3600,
	}).Error)

	items, err := buildAdminUserListItems([]*model.User{&user}, now)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.EqualValues(t, 75, items[0].BonusQuota)
	assert.EqualValues(t, now+3600, items[0].BonusNearestExpiresAt)
	assert.EqualValues(t, 375, items[0].TotalQuota)

	genericJSON, err := json.Marshal(&user)
	require.NoError(t, err)
	assert.NotContains(t, string(genericJSON), "bonus_quota")
	assert.NotContains(t, string(genericJSON), "bonus_nearest_expires_at")
	assert.NotContains(t, string(genericJSON), "total_quota")
	adminJSON, err := json.Marshal(items[0])
	require.NoError(t, err)
	assert.Contains(t, string(adminJSON), `"bonus_quota":75`)
	assert.Contains(t, string(adminJSON), `"total_quota":375`)
}

func TestAdminUserListAndSearchUseOneBonusSelectForAnyPageSize(t *testing.T) {
	tests := []struct {
		name   string
		search bool
	}{
		{name: "list", search: false},
		{name: "search", search: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, pageSize := range []int{1, 2} {
				t.Run(fmt.Sprintf("page_size_%d", pageSize), func(t *testing.T) {
					db := setupManageUserTestDB(t)
					counter := &selectCountingLogger{}
					model.DB = db.Session(&gorm.Session{Logger: counter})
					now := time.Now().Unix()
					users := []model.User{
						{Username: "query-user-one", AffCode: "query-user-one", Quota: 300},
						{Username: "query-user-two", AffCode: "query-user-two", Quota: 400},
					}
					insertManageUserFixture(t, db, 1, &users[0])
					insertManageUserFixture(t, db, 2, &users[1])
					require.NoError(t, db.Create(&model.BonusBalance{
						UserId: users[0].Id, CampaignId: "query-valid", TopUpId: 1,
						Status: model.BonusBalanceStatusActive, AmountTotal: 100,
						AmountUsed: 25, ExpiresAt: now + 3600,
					}).Error)
					require.NoError(t, db.Create(&model.BonusBalance{
						UserId: users[0].Id, CampaignId: "query-expired", TopUpId: 2,
						Status: model.BonusBalanceStatusActive, AmountTotal: 999,
						ExpiresAt: now - 1,
					}).Error)

					gin.SetMode(gin.TestMode)
					recorder := httptest.NewRecorder()
					context, _ := gin.CreateTestContext(recorder)
					path := fmt.Sprintf("/api/user/?p=1&page_size=%d", pageSize)
					if test.search {
						path = fmt.Sprintf("/api/user/search?keyword=query-user&p=1&page_size=%d", pageSize)
					}
					context.Request = httptest.NewRequest(http.MethodGet, path, nil)
					if test.search {
						SearchUsers(context)
					} else {
						GetAllUsers(context)
					}

					assert.Equal(t, http.StatusOK, recorder.Code)
					responseBody := recorder.Body.String()
					assert.Contains(t, responseBody, `"bonus_nearest_expires_at":`)
					if pageSize == 1 {
						assert.Contains(t, responseBody, `"bonus_quota":0`)
						assert.Contains(t, responseBody, `"bonus_nearest_expires_at":0`)
						assert.Contains(t, responseBody, `"total_quota":400`)
					} else {
						assert.Contains(t, responseBody, `"bonus_quota":75`)
						assert.Contains(t, responseBody, fmt.Sprintf(`"bonus_nearest_expires_at":%d`, now+3600))
						assert.Contains(t, responseBody, `"total_quota":375`)
					}
					assert.Equal(t, 3, counter.selectCount, "count, page, and one grouped bonus query expected")
				})
			}
		})
	}
}

func performManageUserRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	ManageUser(c)
	return recorder
}

func TestManageUserDisableAdvancesAuthVersionOnceAndRevokesSession(t *testing.T) {
	db := setupManageUserTestDB(t)
	now := time.Now().Unix()
	user := model.User{
		Username: "managed-disable-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.UserSession{
		SID: "managed-disable-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "password",
		LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"disable"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, updated.Status)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var session model.UserSession
	require.NoError(t, db.First(&session, "sid = ?", "managed-disable-session").Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
}

func TestManageUserDemoteAdvancesAuthVersionAndRevokesSessionsOnce(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	require.NoError(t, authz.Init(db))

	now := time.Now().Unix()
	user := model.User{
		Username: "managed-demote-user", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	for _, sid := range []string{"managed-demote-session-one", "managed-demote-session-two"} {
		require.NoError(t, db.Create(&model.UserSession{
			SID: sid, UserID: user.Id, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusActive, RefreshHash: "refresh-" + sid, LoginMethod: "password",
			LastActiveAt: now, ExpiresAt: now + 3600,
		}).Error)
	}

	sessionUpdateCount := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:count_demote_session_updates", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_sessions" {
			sessionUpdateCount++
		}
	}))

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"demote"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, updated.Role)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var sessions []model.UserSession
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("sid asc").Find(&sessions).Error)
	require.Len(t, sessions, 2)
	for _, session := range sessions {
		assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
		assert.Equal(t, "admin_demote", session.RevokedReason)
	}
	assert.Equal(t, 1, sessionUpdateCount)
}

func TestManageUserDeleteReturnsImmediatelyAndUnknownActionFails(t *testing.T) {
	db := setupManageUserTestDB(t)
	deleted := model.User{
		Username: "managed-delete-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "delete-aff",
	}
	require.NoError(t, db.Create(&deleted).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"delete"}`, deleted.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var deletedCount int64
	require.NoError(t, db.Unscoped().Model(&model.User{}).Where("id = ? AND deleted_at IS NOT NULL", deleted.Id).Count(&deletedCount).Error)
	assert.EqualValues(t, 1, deletedCount)

	unchanged := model.User{
		Username: "managed-unknown-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "unknown-aff",
	}
	require.NoError(t, db.Create(&unchanged).Error)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"unknown"}`, unchanged.Id))
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	require.NoError(t, db.First(&unchanged, unchanged.Id).Error)
	assert.EqualValues(t, 1, unchanged.AuthVersion)
	assert.Equal(t, common.UserStatusEnabled, unchanged.Status)
}

func TestRegistrationAndEmailBindConsumeVerificationCode(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousEmailVerification := common.EmailVerificationEnabled
	previousRegisterEnabled := common.RegisterEnabled
	previousPasswordRegisterEnabled := common.PasswordRegisterEnabled
	previousGenerateDefaultToken := constant.GenerateDefaultToken
	common.EmailVerificationEnabled = true
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	constant.GenerateDefaultToken = false
	t.Cleanup(func() {
		common.EmailVerificationEnabled = previousEmailVerification
		common.RegisterEnabled = previousRegisterEnabled
		common.PasswordRegisterEnabled = previousPasswordRegisterEnabled
		constant.GenerateDefaultToken = previousGenerateDefaultToken
	})

	registrationEmail := "student@example.com"
	registrationCode := "123456"
	common.RegisterVerificationCodeWithKey(registrationEmail, registrationCode, common.EmailVerificationPurpose)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(fmt.Sprintf(
		`{"username":"registration-user","password":"Password123","email":"%s","verification_code":"%s"}`,
		registrationEmail, registrationCode)))
	context.Request.Header.Set("Content-Type", "application/json")
	Register(context)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.False(t, common.VerifyCodeWithKey(registrationEmail, registrationCode, common.EmailVerificationPurpose))

	var user model.User
	require.NoError(t, db.Where("username = ?", "registration-user").First(&user).Error)
	bindEmail := "bound@example.com"
	bindCode := "654321"
	common.RegisterVerificationCodeWithKey(bindEmail, bindCode, common.EmailVerificationPurpose)
	recorder = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(recorder)
	context.Set("id", user.Id)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/email/bind", strings.NewReader(fmt.Sprintf(
		`{"email":" %s ","code":"%s"}`,
		strings.ToUpper(bindEmail), bindCode)))
	context.Request.Header.Set("Content-Type", "application/json")
	EmailBind(context)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.False(t, common.VerifyCodeWithKey(bindEmail, bindCode, common.EmailVerificationPurpose))
}
