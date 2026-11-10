package secrets

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Keychain interface {
	Get(account string) (string, error)
	Set(account, value string) error
	Delete(account string) error
	List() ([]string, error)
}

type securityKeychain struct{}

func DefaultKeychain() Keychain { return securityKeychain{} }

func (securityKeychain) Get(account string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", ServiceName, "-a", account, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("keychain get %s: %w", account, err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (securityKeychain) Set(account, value string) error {
	cmd := exec.Command("security", "add-generic-password", "-U", "-s", ServiceName, "-a", account, "-w", value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain set: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func (securityKeychain) Delete(account string) error {
	cmd := exec.Command("security", "delete-generic-password", "-s", ServiceName, "-a", account)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain rm: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func (securityKeychain) List() ([]string, error) {
	cmd := exec.Command("security", "dump-keychain")
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("security", "find-generic-password", "-s", ServiceName)
		out, err = cmd.CombinedOutput()
		if err != nil {
			return nil, nil
		}
	}
	var names []string
	seen := map[string]struct{}{}
	lines := strings.Split(string(out), "\n")
	svcOK := false
	for _, line := range lines {
		if strings.Contains(line, `"svce"`) && strings.Contains(line, ServiceName) {
			svcOK = true
		}
		if strings.Contains(line, `"acct"`) {
			if i := strings.Index(line, "<blob>="); i >= 0 {
				name := strings.Trim(strings.TrimSpace(line[i+7:]), `"`)
				if svcOK && name != "" {
					if _, ok := seen[name]; !ok {
						seen[name] = struct{}{}
						names = append(names, name)
					}
				}
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "keychain:") {
			svcOK = false
		}
	}
	return names, nil
}

func KeychainAvailable() bool {
	_, err := exec.LookPath("security")
	return err == nil
}
