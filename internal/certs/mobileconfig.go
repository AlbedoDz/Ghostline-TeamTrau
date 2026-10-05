package certs

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"net/netip"
	"text/template"
)

// ErrNoSSID means the iOS profile cannot be built without the home Wi-Fi
// name: without OnDemandRules it would break DNS away from home.
var ErrNoSSID = errors.New("certs: the iOS profile needs a Wi-Fi name")

// ProfileInput describes the iOS DoH profile.
type ProfileInput struct {
	CA    *CA
	Addrs []netip.Addr // LAN addresses of this PC (ServerAddresses)
	Port  int          // DoH port; 443 is left out of the URL
	SSID  string       // home Wi-Fi; DoH is used only there
}

// MobileConfig builds an unsigned .mobileconfig with the LAN CA and a DoH
// DNS payload limited to SSID by OnDemandRules (spec 2B 7.3).
func MobileConfig(in ProfileInput) ([]byte, error) {
	if in.SSID == "" {
		return nil, ErrNoSSID
	}
	url := "https://dns." + LANDomain
	if in.Port != 0 && in.Port != 443 {
		url += fmt.Sprintf(":%d", in.Port)
	}
	url += "/dns-query"
	addrs := make([]string, len(in.Addrs))
	for i, a := range in.Addrs {
		addrs[i] = a.String()
	}
	thumb := in.CA.Thumbprint()
	data := map[string]any{
		"CA":       base64.StdEncoding.EncodeToString(in.CA.DER),
		"CAName":   in.CA.Cert.Subject.CommonName,
		"URL":      url,
		"Addrs":    addrs,
		"SSID":     in.SSID,
		"UUIDRoot": uuidFrom(thumb, "root"),
		"UUIDDNS":  uuidFrom(thumb, "dns"),
		"UUIDTop":  uuidFrom(thumb, "profile"),
		"ID":       "lan.ghostline." + thumb[:12],
	}
	var buf bytes.Buffer
	if err := profileTmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// uuidFrom derives a stable UUID so re-installing the profile for the same
// CA replaces the old one instead of adding a second.
func uuidFrom(thumb, kind string) string {
	s := sha256.Sum256([]byte(thumb + "/" + kind))
	return fmt.Sprintf("%X-%X-%X-%X-%X", s[0:4], s[4:6], s[6:8], s[8:10], s[10:16])
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

var profileTmpl = template.Must(template.New("p").Funcs(template.FuncMap{"x": xmlEscape}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadCertificateFileName</key>
			<string>ghostline-lan-ca.crt</string>
			<key>PayloadContent</key>
			<data>{{.CA}}</data>
			<key>PayloadDisplayName</key>
			<string>{{x .CAName}}</string>
			<key>PayloadIdentifier</key>
			<string>{{.ID}}.root</string>
			<key>PayloadType</key>
			<string>com.apple.security.root</string>
			<key>PayloadUUID</key>
			<string>{{.UUIDRoot}}</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
		<dict>
			<key>DNSSettings</key>
			<dict>
				<key>DNSProtocol</key>
				<string>HTTPS</string>
				<key>ServerURL</key>
				<string>{{x .URL}}</string>
				<key>ServerAddresses</key>
				<array>{{range .Addrs}}
					<string>{{x .}}</string>{{end}}
				</array>
			</dict>
			<key>OnDemandRules</key>
			<array>
				<dict>
					<key>Action</key>
					<string>Connect</string>
					<key>InterfaceTypeMatch</key>
					<string>WiFi</string>
					<key>SSIDMatch</key>
					<array>
						<string>{{x .SSID}}</string>
					</array>
				</dict>
				<dict>
					<key>Action</key>
					<string>Disconnect</string>
				</dict>
			</array>
			<key>PayloadDisplayName</key>
			<string>Ghostline DNS</string>
			<key>PayloadIdentifier</key>
			<string>{{.ID}}.dns</string>
			<key>PayloadType</key>
			<string>com.apple.dnsSettings.managed</string>
			<key>PayloadUUID</key>
			<string>{{.UUIDDNS}}</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>Ghostline DNS</string>
	<key>PayloadIdentifier</key>
	<string>{{.ID}}</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>{{.UUIDTop}}</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`))
