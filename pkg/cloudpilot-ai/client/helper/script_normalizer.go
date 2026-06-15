package helper

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	managedAssignmentPattern = regexp.MustCompile(`^(export[ \t]+)?([A-Z_][A-Z0-9_]*)=(?:"[^"]*"|'[^']*'|[^;[:space:]]+)([ \t]*;[ \t]*|[ \t]+|$)`)
	managedUnsetPattern      = regexp.MustCompile(`^unset[ \t]+([A-Z_][A-Z0-9_]*(?:[ \t]+[A-Z_][A-Z0-9_]*)*)([ \t]*;[ \t]*|$)`)
	awsCommandPrefixPattern  = regexp.MustCompile(`^(?:[A-Z_][A-Z0-9_]*=(?:"[^"]*"|'[^']*'|[^;[:space:]]+)[ \t]+)*aws(?:[ \t]+|$)`)
	awsProfileArgPattern     = regexp.MustCompile(`(^|[ \t])--profile(?:=(?:"[^"]*"|'[^']*'|[^ \t;]+)|[ \t]+(?:"[^"]*"|'[^']*'|[^ \t;]+))`)
)

var assumeRoleManagedEnvNames = []string{
	"AWS_PROFILE",
	"AWS_DEFAULT_PROFILE",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_ROLE_ARN",
	"AWS_WEB_IDENTITY_TOKEN_FILE",
}

type ManagedScriptOverrideConfig struct {
	StripEnvNames       map[string]struct{}
	StripAWSProfileFlag bool
}

func NewManagedScriptOverrideConfig(extra, awsEnv map[string]string) ManagedScriptOverrideConfig {
	names := map[string]struct{}{
		"AWS_PROFILE":         {},
		"AWS_DEFAULT_PROFILE": {},
	}
	hasTempCreds := hasTemporaryAWSCredentials(awsEnv)

	if hasTempCreds {
		for _, name := range assumeRoleManagedEnvNames {
			names[name] = struct{}{}
		}
	}

	if extra["CUSTOM_NODE_ROLE"] != "" {
		names["CUSTOM_NODE_ROLE"] = struct{}{}
	}

	return ManagedScriptOverrideConfig{
		StripEnvNames:       names,
		StripAWSProfileFlag: true,
	}
}

func hasTemporaryAWSCredentials(env map[string]string) bool {
	return env["AWS_ACCESS_KEY_ID"] != "" &&
		env["AWS_SECRET_ACCESS_KEY"] != "" &&
		env["AWS_SESSION_TOKEN"] != ""
}

func NormalizeManagedScript(script string, cfg ManagedScriptOverrideConfig) string {
	if len(cfg.StripEnvNames) == 0 && !cfg.StripAWSProfileFlag {
		return script
	}

	lines := strings.Split(script, "\n")
	for i, line := range lines {
		lines[i] = normalizeManagedScriptLine(line, cfg)
	}

	return strings.Join(lines, "\n")
}

func normalizeManagedScriptLine(line string, cfg ManagedScriptOverrideConfig) string {
	indentLen := len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
	indent := line[:indentLen]
	body := line[indentLen:]

	if strings.TrimSpace(body) == "" {
		return line
	}
	if hasManagedContinuationAssignment(body, cfg.StripEnvNames) {
		return line
	}

	body = stripManagedUnset(body, cfg.StripEnvNames)
	body = stripManagedLeadingAssignments(body, cfg.StripEnvNames)
	if cfg.StripAWSProfileFlag {
		body = stripAWSCLIProfileFlag(body)
	}

	return indent + body
}

func hasManagedContinuationAssignment(body string, managed map[string]struct{}) bool {
	trimmedRight := strings.TrimRightFunc(body, unicode.IsSpace)
	if !strings.HasSuffix(trimmedRight, `\`) {
		return false
	}

	matches := managedAssignmentPattern.FindStringSubmatch(body)
	if matches == nil {
		return false
	}

	_, ok := managed[matches[2]]
	return ok
}

func stripManagedUnset(body string, managed map[string]struct{}) string {
	matches := managedUnsetPattern.FindStringSubmatch(body)
	if matches == nil {
		return body
	}

	names := strings.Fields(matches[1])
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := managed[name]; ok {
			continue
		}
		kept = append(kept, name)
	}

	suffix := strings.TrimLeft(strings.TrimPrefix(body, matches[0]), " \t")
	switch {
	case len(kept) == 0:
		return suffix
	case suffix == "":
		return "unset " + strings.Join(kept, " ")
	default:
		return "unset " + strings.Join(kept, " ") + " " + suffix
	}
}

func stripManagedLeadingAssignments(body string, managed map[string]struct{}) string {
	kept := make([]string, 0, 2)
	rest := body

	for {
		matches := managedAssignmentPattern.FindStringSubmatch(rest)
		if matches == nil {
			break
		}

		full := matches[0]
		name := matches[2]
		if _, ok := managed[name]; ok {
			rest = strings.TrimLeft(strings.TrimPrefix(rest, full), " \t")
			continue
		}

		kept = append(kept, strings.TrimSpace(full))
		rest = strings.TrimLeft(strings.TrimPrefix(rest, full), " \t")
	}

	if len(kept) == 0 {
		return rest
	}
	if rest == "" {
		return strings.Join(kept, " ")
	}

	return strings.Join(append(kept, rest), " ")
}

func stripAWSCLIProfileFlag(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return body
	}
	if !awsCommandPrefixPattern.MatchString(trimmed) {
		return body
	}

	return strings.TrimSpace(awsProfileArgPattern.ReplaceAllString(trimmed, "$1"))
}
