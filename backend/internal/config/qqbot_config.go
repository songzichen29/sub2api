package config

// QQBotNapCatConfig is operator configuration, never part of the public bot
// settings. The WebUI credential grants more access than the OneBot token.
type QQBotNapCatConfig struct {
	WebUIURL       string `mapstructure:"webui_url"`
	WebUIToken     string `mapstructure:"webui_token"`
	WebUITokenFile string `mapstructure:"webui_token_file"`
}
