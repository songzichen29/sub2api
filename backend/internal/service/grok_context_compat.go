package service

import "github.com/gin-gonic/gin"

const (
	grokConversationIDHeader = "X-Grok-Conv-Id"
	claudeCodeSessionHeader  = "X-Claude-Code-Session-Id"
)

// isGrokRequestContext limits Grok-specific session headers to native Grok
// groups or composite routes that have already resolved to Grok.
func isGrokRequestContext(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if c.Request != nil {
		if platform, ok := ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
			return platform == PlatformGrok
		}
	}
	value, exists := c.Get("api_key")
	if !exists {
		return false
	}
	apiKey, ok := value.(*APIKey)
	return ok && apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == PlatformGrok
}

func (a *Account) IsGrok() bool {
	return a != nil && a.Platform == PlatformGrok
}

func (a *Account) IsGrokOAuth() bool {
	return a.IsGrok() && a.Type == AccountTypeOAuth
}

func (a *Account) IsOpenAICompatible() bool {
	return a != nil && (a.Platform == PlatformOpenAI || a.Platform == PlatformGrok)
}
