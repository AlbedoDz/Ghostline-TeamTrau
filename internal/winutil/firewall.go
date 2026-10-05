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

func firewallShowArgs() []string { return showArgs(FirewallRuleName) }

func showArgs(name string) []string {
	return []string{"advfirewall", "firewall", "show", "rule", "name=" + name}
}

// Names of the DNS server rules (spec 2B 6.4).
const (
	RuleDNSTCP = "Ghostline DNS (TCP)"
	RuleDNSUDP = "Ghostline DNS (UDP)"
	RuleSetup  = "Ghostline Setup"
	// RuleBlockPublic blocks every inbound connection to Ghostline on
	// Public networks. Block wins over Allow, so the Allow rules Windows
	// creates when someone answers its firewall prompt cannot open the
	// proxy or DNS server on public Wi-Fi.
	RuleBlockPublic = "Ghostline Block Public"
)

// BlockPublicRule is the Public-profile block rule.
var BlockPublicRule = FirewallRule{Name: RuleBlockPublic, Block: true}

// AllRuleNames lists every inbound rule Ghostline may create, for cleanup.
var AllRuleNames = []string{FirewallRuleName, RuleDNSTCP, RuleDNSUDP, RuleSetup, RuleBlockPublic}

// FirewallRule is a named inbound rule for some local ports.
type FirewallRule struct {
	Name     string
	Protocol string // TCP | UDP
	Ports    []int
	// Block makes a Public-profile block rule for the exe (all protocols
	// and ports); Protocol and Ports are ignored.
	Block bool
}

// FirewallRuleArgs are the netsh arguments for r: only Ghostline's exe,
// only Private networks, only the local subnet.
func FirewallRuleArgs(r FirewallRule, exe string) []string {
	if r.Block {
		return []string{"advfirewall", "firewall", "add", "rule", "name=" + r.Name, "dir=in", "action=block",
			"program=" + exe, "profile=public"}
	}
	ports := make([]string, len(r.Ports))
	for i, p := range r.Ports {
		ports[i] = strconv.Itoa(p)
	}
	return []string{"advfirewall", "firewall", "add", "rule", "name=" + r.Name, "dir=in", "action=allow",
		"protocol=" + r.Protocol, "localport=" + strings.Join(ports, ","), "program=" + exe, "profile=private", "remoteip=localsubnet"}
}

// AddNamedRule (re)creates r.
func AddNamedRule(r FirewallRule, exe string) error {
	if err := DeleteNamedRule(r.Name); err != nil {
		return err
	}
	if out, err := runNetsh(FirewallRuleArgs(r, exe)); err != nil {
		return fmt.Errorf("firewall: add rule %q: %v: %s", r.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteNamedRule removes the rule called name; a missing rule is not an
// error (see DeleteFirewallRule).
func DeleteNamedRule(name string) error {
	out, err := runNetsh([]string{"advfirewall", "firewall", "delete", "rule", "name=" + name})
	if err == nil {
		return nil
	}
	if _, serr := runNetsh(showArgs(name)); serr != nil {
		return nil // no such rule
	}
	return fmt.Errorf("firewall: delete rule %q: %v: %s", name, err, strings.TrimSpace(string(out)))
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
