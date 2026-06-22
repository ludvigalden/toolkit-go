package supervisor

import (
	"fmt"
	"strings"
)

// Identity returns the reverse-DNS service identity namespace.tool.port
// that names both the systemd unit and the launchd label.
func Identity(namespace, tool string, port int) string {
	return fmt.Sprintf("%s.%s.%d", namespace, tool, port)
}

// Tool returns the tool segment of UnitName: everything before the first
// colon.
func (s Spec) Tool() (string, error) {
	tool, _, _ := strings.Cut(s.UnitName, ":")
	if err := checkSegment("unit name tool segment", tool); err != nil {
		return "", err
	}
	return tool, nil
}

// Identity returns Identity(s.Namespace, s.Tool(), s.Port), validating
// that both segments are safe in file names and unit files.
func (s Spec) Identity() (string, error) {
	if err := checkSegment("namespace", s.Namespace); err != nil {
		return "", err
	}
	tool, err := s.Tool()
	if err != nil {
		return "", err
	}
	return Identity(s.Namespace, tool, s.Port), nil
}

// checkSegment admits [A-Za-z0-9._-]: identities become file names and
// unit-file values, so separators and whitespace are refused.
func checkSegment(field, v string) error {
	if v == "" {
		return fmt.Errorf("supervisor: spec %s is empty", field)
	}
	for _, r := range v {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '.' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("supervisor: spec %s %q contains %q", field, v, r)
		}
	}
	return nil
}
