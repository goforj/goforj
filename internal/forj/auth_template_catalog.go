package forj

// authBaseTemplates shares the auth render surface with the upgrade inspector so ownership cannot drift between them.
func authBaseTemplates() []string {
	return []string{
		"internal/mail/auth_delivery.go.tmpl",
		"internal/auth/README.md.tmpl",
		"internal/auth/controller.go.tmpl",
		"internal/auth/delivery.go.tmpl",
		"internal/auth/bootstrap_cmd.go.tmpl",
		"internal/auth/create_user_cmd.go.tmpl",
		"internal/auth/email_verification.go.tmpl",
		"internal/auth/login_attempt.go.tmpl",
		"internal/auth/service_integration_test.go.tmpl",
		"internal/auth/password_reset.go.tmpl",
		"internal/auth/set_password_cmd.go.tmpl",
		"internal/auth/service.go.tmpl",
		"internal/auth/session.go.tmpl",
		"internal/auth/user.go.tmpl",
	}
}

// authOAuthTemplates keeps optional provider ownership identical for rendering and comparison.
func authOAuthTemplates() []string {
	return []string{
		"internal/auth/identity.go.tmpl",
		"internal/auth/oauth_provider.go.tmpl",
		"internal/auth/oauth_provider_apple.go.tmpl",
		"internal/auth/oauth_provider_github.go.tmpl",
		"internal/auth/oauth_provider_google.go.tmpl",
		"internal/auth/oauth_provider_microsoft.go.tmpl",
		"internal/auth/oauth_integration_test.go.tmpl",
		"internal/auth/oauth_state.go.tmpl",
	}
}
