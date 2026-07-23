package wire

// provideEmptyInheritedEnvironment leaves runtime commands without a wired launcher snapshot for compatibility callers.
func provideEmptyInheritedEnvironment() map[string]string {
	return nil
}
