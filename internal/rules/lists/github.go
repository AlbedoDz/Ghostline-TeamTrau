package lists

import (
	"errors"
	"net/url"
	"strings"
)

// NormalizeURL turns any GitHub-ish link (blob, raw, jsDelivr, gist) into a
// raw download URL. For GitHub files, fallback is the jsDelivr mirror; it is
// "" otherwise. Only HTTPS is accepted; a missing scheme means https.
func NormalizeURL(raw string) (primary, fallback string, err error) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", errors.New("lists: only https:// links are supported")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	gh := func(user, repo, ref string, rest []string) (string, string, error) {
		p := strings.Join(rest, "/")
		return "https://raw.githubusercontent.com/" + user + "/" + repo + "/" + ref + "/" + p,
			"https://cdn.jsdelivr.net/gh/" + user + "/" + repo + "@" + ref + "/" + p, nil
	}
	switch strings.ToLower(u.Host) {
	case "github.com", "www.github.com":
		if len(parts) >= 5 && (parts[2] == "blob" || parts[2] == "raw") {
			return gh(parts[0], parts[1], parts[3], parts[4:])
		}
	case "raw.githubusercontent.com":
		if len(parts) >= 6 && parts[2] == "refs" && (parts[3] == "heads" || parts[3] == "tags") {
			return gh(parts[0], parts[1], parts[4], parts[5:])
		}
		if len(parts) >= 4 {
			return gh(parts[0], parts[1], parts[2], parts[3:])
		}
	case "cdn.jsdelivr.net":
		if len(parts) >= 4 && parts[0] == "gh" {
			repo, ref, ok := strings.Cut(parts[2], "@")
			if ok {
				return gh(parts[1], repo, ref, parts[3:])
			}
		}
	case "gist.github.com":
		if len(parts) == 2 {
			return u.String() + "/raw", "", nil
		}
	}
	return u.String(), "", nil
}
