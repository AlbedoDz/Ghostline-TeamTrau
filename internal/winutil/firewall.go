package winutil

import (
	"fmt"
	"strconv"
	"strings"
)

// FirewallRuleName is the fixed name of the inbound rule for LAN sharing, so
// cleanup can always find it.
const FirewallRuleName = "Ghostline Proxy"

// FirewallAddArgs are the netsh arguments for the LAN-sharing rule: only the
// proxy port, only Ghostline's exe, only Private networks, only the local
// subnet.
func FirewallAddArgs(port int, exe string) []string {
	return []string{"advfirewall", "firewall", "add", "rule", "name=" + FirewallRuleName, "dir=in", "action=allow",
		"protocol=TCP", "localport=" + strconv.Itoa(port), "program=" + exe, "profile=private", "remoteip=localsubnet"}
}

// FirewallDeleteArgs removes the rule.
func FirewallDeleteArgs() []string {
	return []string{"advfirewall", "firewall", "delete", "rule", "name=" + FirewallRuleName}
}

func firewallShowArgs() []string {
	return []string{"advfirewall", "firewall", "show", "rule", "name=" + FirewallRuleName}
}

// runNetsh runs netsh with args (replaced in tests).
var runNetsh = netsh

// AddFirewallRule (re)creates the LAN-sharing rule.
func AddFirewallRule(port int, exe string) error {
	if err := DeleteFirewallRule(); err != nil {
		return err
	}
	if out, err := runNetsh(FirewallAddArgs(port, exe)); err != nil {
		return fmt.Errorf("firewall: add rule: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteFirewallRule removes the rule; a missing rule is not an error.
// netsh's messages are localised, so "missing" is decided by a failing
// "show rule" rather than by parsing text.
func DeleteFirewallRule() error {
	out, err := runNetsh(FirewallDeleteArgs())
	if err == nil {
		return nil
	}
	if _, serr := runNetsh(firewallShowArgs()); serr != nil {
		return nil // no such rule
	}
	return fmt.Errorf("firewall: delete rule: %v: %s", err, strings.TrimSpace(string(out)))
}

// parsePublic reports whether any network category line is "Public".
func parsePublic(out string) bool {
	for _, l := range strings.Split(out, "\n") {
		if strings.EqualFold(strings.TrimSpace(l), "Public") {
			return true
		}
	}
	return false
}
