// Package model holds plain data types shared across Ghostline packages.
package model

// Protocol is an encrypted DNS transport.
type Protocol string

const (
	ProtoDoH      Protocol = "doh"
	ProtoDoT      Protocol = "dot"
	ProtoDoQ      Protocol = "doq"
	ProtoDNSCrypt Protocol = "dnscrypt"
)

// Source says where a server entry came from.
type Source string

const (
	SourceBuiltin  Source = "builtin"
	SourceRemote   Source = "remote"
	SourceDNSCrypt Source = "dnscrypt"
	SourceCustom   Source = "custom"
)

// Server is one upstream DNS resolver.
type Server struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Provider string   `json:"provider"`
	Protocol Protocol `json:"protocol"`
	Address  string   `json:"address"`
	IPs      []string `json:"ips,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Source   Source   `json:"source"`
}

// DNSMode says whether an adapter's DNS came from DHCP or was set statically.
type DNSMode string

const (
	DNSModeDHCP   DNSMode = "dhcp"
	DNSModeStatic DNSMode = "static"
)

// FamilyDNS is the DNS configuration of one address family on one adapter.
type FamilyDNS struct {
	Mode    DNSMode  `json:"mode"`
	Servers []string `json:"servers,omitempty"`
}

// AdapterSnapshot records an adapter's DNS before Ghostline changed it.
type AdapterSnapshot struct {
	GUID    string    `json:"guid"`
	LUID    uint64    `json:"luid"`
	IfIndex uint32    `json:"ifIndex"`
	Alias   string    `json:"alias"`
	IPv4    FamilyDNS `json:"ipv4"`
	IPv6    FamilyDNS `json:"ipv6"`
}
