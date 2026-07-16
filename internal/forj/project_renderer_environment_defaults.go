package forj

import (
	"fmt"
	"strings"

	"github.com/goforj/crypt"
	"github.com/goforj/goforj/internal/envfile"
)

// environmentAssignment retains the decoded value and source position of one controlling dotenv assignment.
type environmentAssignment struct {
	index int
	value string
}

// exists reports whether an active assignment was found without conflating absence with an intentionally empty owner value.
func (assignment environmentAssignment) exists() bool {
	return assignment.index >= 0
}

// ensureEnvironmentDefaults fills missing framework values while leaving concrete owner assignments byte-for-byte intact.
func (p *ProjectRenderer) ensureEnvironmentDefaults(path string) error {
	content, err := p.workspace.readFile(path)
	if err != nil {
		return err
	}
	text := string(content)
	lines := strings.Split(text, "\n")
	lines, duplicateSecretRemoved := removeDuplicateEnvironmentAssignments(lines, "LIGHTHOUSE_SECRET")

	appKey := finalEnvironmentAssignment(lines, "APP_KEY")
	appDiagToken := finalEnvironmentAssignment(lines, "APP_DIAG_TOKEN")
	lighthouseSecret := finalEnvironmentAssignment(lines, "LIGHTHOUSE_SECRET")
	jwtSecret := finalEnvironmentAssignment(lines, "API_JWT_SECRET_KEY")
	isOwnerEnvironment := path == ".env"
	needsURL := isOwnerEnvironment && !finalEnvironmentAssignment(lines, "LIGHTHOUSE_URL").exists()
	needsAppDiagToken := isOwnerEnvironment && !appDiagToken.exists()
	needsSecret := isOwnerEnvironment && !lighthouseSecret.exists()
	needsEnabled := isOwnerEnvironment && !finalEnvironmentAssignment(lines, "LIGHTHOUSE_ENABLED").exists()
	needsSwagger := isOwnerEnvironment && !finalEnvironmentAssignment(lines, "SWAGGER_ENABLED").exists()
	needsForjMakeOpen := isOwnerEnvironment && !finalEnvironmentAssignment(lines, "FORJ_MAKE_OPEN").exists()
	needsForjEditor := isOwnerEnvironment && !finalEnvironmentAssignment(lines, "FORJ_EDITOR").exists()
	needsKey := isOwnerEnvironment && !appKey.exists()
	needsJWTSecret := isOwnerEnvironment && (!jwtSecret.exists() || strings.TrimSpace(jwtSecret.value) == "" || strings.TrimSpace(jwtSecret.value) == "xxx")

	needsGrafanaPortDefault := false
	if isOwnerEnvironment && p.config.Render.Components.Grafana {
		lines, needsGrafanaPortDefault = migrateGeneratedEnvDefault(lines, "GRAFANA_PORT", "3001", "13001")
	}
	if !(duplicateSecretRemoved || needsURL || needsAppDiagToken || needsSecret || needsEnabled || needsSwagger || needsForjMakeOpen || needsForjEditor || needsGrafanaPortDefault || needsKey || needsJWTSecret) {
		return nil
	}

	appKeyValue := ""
	if needsKey {
		appKeyValue, err = crypt.GenerateAppKey()
		if err != nil {
			return fmt.Errorf("failed to generate app key: %w", err)
		}
	}
	appDiagTokenValue := ""
	if needsAppDiagToken {
		appDiagTokenValue, err = generateAppDiagToken()
		if err != nil {
			return fmt.Errorf("failed to generate app diagnostics token: %w", err)
		}
	}
	lighthouseSecretValue := ""
	if needsSecret {
		lighthouseSecretValue, err = generateLighthouseSecret()
		if err != nil {
			return fmt.Errorf("failed to generate lighthouse secret: %w", err)
		}
	}
	jwtSecretValue := ""
	if needsJWTSecret {
		jwtSecretValue, err = generateJWTSecretKey()
		if err != nil {
			return fmt.Errorf("failed to generate JWT secret: %w", err)
		}
	}

	writeLines := make([]string, 0)
	if needsKey {
		writeLines = append(writeLines, "APP_KEY="+appKeyValue)
	}
	if needsAppDiagToken {
		writeLines = append(writeLines, "APP_DIAG_TOKEN="+appDiagTokenValue)
	}
	if needsURL {
		writeLines = append(writeLines, "LIGHTHOUSE_URL=ws://localhost:3000/lighthouse/ws/agent")
	}
	if needsSecret {
		writeLines = append(writeLines, "LIGHTHOUSE_SECRET="+lighthouseSecretValue)
	}
	if needsEnabled {
		writeLines = append(writeLines, "LIGHTHOUSE_ENABLED=true")
	}
	if needsSwagger {
		writeLines = append(writeLines, "SWAGGER_ENABLED=true")
	}
	if needsForjMakeOpen || needsForjEditor {
		if len(writeLines) > 0 {
			writeLines = append(writeLines, "")
		}
		if !strings.Contains(text, "# Forj") {
			writeLines = append(writeLines, "# Forj")
		}
		if needsForjMakeOpen {
			writeLines = append(writeLines, "FORJ_MAKE_OPEN=auto # options: auto, always, never")
		}
		if needsForjEditor {
			writeLines = append(writeLines, "# Optional editor command for make commands; falls back to common GUI editors.")
			writeLines = append(writeLines, "FORJ_EDITOR=")
		}
	}
	if needsJWTSecret {
		line := "API_JWT_SECRET_KEY=" + jwtSecretValue
		if jwtSecret.exists() {
			lines[jwtSecret.index] = line
		} else {
			writeLines = append(writeLines, line)
		}
	}
	if len(writeLines) > 0 {
		lines = append(lines, writeLines...)
	}
	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if updated == text {
		return nil
	}
	return p.workspace.writeFileAtomically(path, []byte(updated), 0o644)
}

// finalEnvironmentAssignment follows dotenv override precedence while ignoring comments and similarly named keys.
func finalEnvironmentAssignment(lines []string, key string) environmentAssignment {
	assignment := environmentAssignment{index: -1}
	for index, line := range lines {
		parsedKey, _, ok := envfile.ParseAssignment(line)
		if !ok || parsedKey != key {
			continue
		}
		assignment.index = index
	}
	if assignment.exists() {
		if value, found := envfile.Lookup(lines, key); found {
			assignment.value = value
		}
	}
	return assignment
}

// removeDuplicateEnvironmentAssignments keeps the first active owner value and removes later generated duplicates without touching comments.
func removeDuplicateEnvironmentAssignments(lines []string, key string) ([]string, bool) {
	seen := false
	changed := false
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		parsedKey, _, ok := envfile.ParseAssignment(line)
		if !ok || parsedKey != key {
			filtered = append(filtered, line)
			continue
		}
		if seen {
			changed = true
			continue
		}
		seen = true
		filtered = append(filtered, line)
	}
	if !changed {
		return lines, false
	}
	return filtered, true
}

// migrateGeneratedEnvDefault updates the controlling old generated default without overriding a custom App owner value.
func migrateGeneratedEnvDefault(lines []string, key string, oldValue string, newValue string) ([]string, bool) {
	assignment := finalEnvironmentAssignment(lines, key)
	if !assignment.exists() {
		return lines, false
	}
	assignmentKey, assignmentValue, found := strings.Cut(strings.TrimSpace(lines[assignment.index]), "=")
	if !found || strings.TrimSpace(assignmentKey) != key || strings.TrimSpace(assignmentValue) != oldValue {
		return lines, false
	}
	updated := append([]string(nil), lines...)
	updated[assignment.index] = key + "=" + newValue
	return updated, true
}
