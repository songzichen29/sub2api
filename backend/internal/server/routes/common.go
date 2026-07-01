package routes

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	registerClaudeCodeCompatibilityRoutes(r)

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}

func registerClaudeCodeCompatibilityRoutes(r *gin.Engine) {
	// Claude Code 1P telemetry endpoints. The gateway itself forwards synthetic
	// telemetry upstream; client-originated telemetry posted to sub2api should not
	// 404 or block the CLI.
	telemetryOK := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	r.POST("/api/event_logging/batch", telemetryOK)
	r.POST("/api/event_logging/v2/batch", telemetryOK)

	// GrowthBook SDK endpoints. Remote eval returns an empty but valid feature
	// payload so the CLI initialization path completes and future feature access
	// has a deterministic "no experiment assigned" state.
	features := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"features":    gin.H{},
			"savedGroups": gin.H{},
			"dateUpdated": time.Now().UTC().Format(time.RFC3339),
		})
	}
	r.GET("/api/features/:clientKey", features)
	r.POST("/api/eval/:clientKey", features)
	r.GET("/sub/:clientKey", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	nowUTC := func() string { return time.Now().UTC().Format(time.RFC3339) }
	emptyUserSettings := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"userId":       "",
			"version":      0,
			"lastModified": nowUTC(),
			"checksum":     "",
			"content": gin.H{
				"entries": gin.H{},
			},
		})
	}
	emptyTeamMemory := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"organizationId": "",
			"repo":           c.Query("repo"),
			"version":        0,
			"lastModified":   nowUTC(),
			"checksum":       "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"content": gin.H{
				"entries":        gin.H{},
				"entryChecksums": gin.H{},
			},
		})
	}
	okJSON := func(payload gin.H) gin.HandlerFunc {
		return func(c *gin.Context) { c.JSON(http.StatusOK, payload) }
	}
	noContent := func(c *gin.Context) { c.Status(http.StatusNoContent) }

	// Claude Code bootstrap/profile/settings/policy/usage endpoints observed in
	// v2.1.197. Responses are intentionally minimal but schema-compatible.
	r.GET("/api/claude_cli/bootstrap", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"client_data":              gin.H{},
			"additional_model_options": []gin.H{},
		})
	})
	r.GET("/api/claude_cli_profile", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"account":      gin.H{},
			"organization": gin.H{},
		})
	})
	r.GET("/api/oauth/profile", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"account":      gin.H{},
			"organization": gin.H{},
		})
	})
	r.GET("/api/oauth/claude_cli/roles", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"organization_role": "",
			"workspace_role":    "",
			"organization_name": "",
		})
	})
	r.GET("/api/oauth/usage", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})
	r.GET("/api/claude_code/settings", noContent)
	r.GET("/api/claude_code/user_settings", emptyUserSettings)
	r.PUT("/api/claude_code/user_settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "checksum": "", "lastModified": nowUTC()})
	})
	r.GET("/api/claude_code/team_memory", emptyTeamMemory)
	r.PUT("/api/claude_code/team_memory", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "checksum": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "lastModified": nowUTC()})
	})
	r.GET("/api/claude_code/memory", okJSON(gin.H{"entries": gin.H{}}))
	r.PUT("/api/claude_code/memory", okJSON(gin.H{"ok": true}))
	r.POST("/api/claude_code/metrics", telemetryOK)
	r.GET("/api/claude_code/organizations/metrics_enabled", okJSON(gin.H{"metrics_enabled": false}))
	r.GET("/api/claude_code/discovery/team_usage", okJSON(gin.H{"usage": []gin.H{}}))
	r.GET("/api/claude_code/skills", okJSON(gin.H{"skills": []gin.H{}}))
	r.GET("/api/claude_code/policy_limits", okJSON(gin.H{"restrictions": gin.H{}}))
	r.GET("/api/claude_code/notification/preferences", okJSON(gin.H{}))
	r.POST("/api/claude_code/notification/preferences", okJSON(gin.H{"ok": true}))
	r.GET("/api/organization/claude_code_first_token_date", okJSON(gin.H{"first_token_date": nil}))
	r.GET("/api/oauth/account/settings", okJSON(gin.H{"grove_enabled": nil, "grove_notice_viewed_at": nil}))
	r.PUT("/api/oauth/account/settings", okJSON(gin.H{"ok": true}))
	r.POST("/api/oauth/account/grove_notice_viewed", okJSON(gin.H{"ok": true}))
	r.GET("/api/claude_code_grove", okJSON(gin.H{"grove_enabled": false, "domain_excluded": false, "notice_is_grace_period": false, "notice_reminder_frequency": nil}))
	r.GET("/api/claude_code_penguin_mode", okJSON(gin.H{"enabled": false}))
	r.GET("/api/oauth/validate", okJSON(gin.H{"valid": true}))
	r.POST("/api/oauth/file_upload", okJSON(gin.H{"ok": false, "files": []gin.H{}}))
	r.GET("/api/oauth/files/:fileUUID/content", noContent)
	r.GET("/api/oauth/organizations", okJSON(gin.H{"organizations": []gin.H{}}))
	r.GET("/api/oauth/organizations/", okJSON(gin.H{"organizations": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/admin_requests", okJSON(gin.H{"admin_requests": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/admin_requests/me", okJSON(gin.H{"admin_requests": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/admin_requests/eligibility", okJSON(gin.H{"eligible": false}))
	r.GET("/api/oauth/organizations/:orgUUID/referral/eligibility", okJSON(gin.H{"eligible": false}))
	r.GET("/api/oauth/organizations/:orgUUID/referral/redemptions", okJSON(gin.H{"redemptions": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/skills/", okJSON(gin.H{"skills": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/skills/list-skills", okJSON(gin.H{"skills": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/plugins/", okJSON(gin.H{"plugins": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/plugins/list-plugins", okJSON(gin.H{"plugins": []gin.H{}}))
	r.GET("/api/oauth/organizations/:orgUUID/sync/github/auth", okJSON(gin.H{"authenticated": false}))
	r.GET("/api/rate-limits", okJSON(gin.H{}))
	r.GET("/api/web/domain_info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"domain": c.Query("domain"), "status": "unknown"})
	})
	r.POST("/api/claude_cli_feedback", okJSON(gin.H{"feedback_id": nil}))
}
