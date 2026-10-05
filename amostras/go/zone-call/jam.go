// Amostra do Zone. Arquivo original: gateway-go/internal/realtime/jam.go
// O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

package realtime

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// CallJam is a Spotify Jam (or other Spotify link) shared with the people in a
// call. Spotify has no public Jam API: the Jam itself runs in each person's
// Spotify app and the call only carries the invitation link.
type CallJam struct {
	URL   string `json:"url"`
	User  string `json:"user_id"`
	Name  string `json:"display_name"`
	Since int64  `json:"since"`
}

var (
	spotifyShortPath   = regexp.MustCompile(`^/[A-Za-z0-9]{4,40}$`)
	spotifyContentPath = regexp.MustCompile(`^(/intl-[a-z]{2}(-[a-z]{2})?)?/(socialsession|jam|playlist|album|track|episode|show|artist)/[A-Za-z0-9]{6,64}/?$`)
)

// normalizeJamURL accepts only HTTPS Spotify invitation/content links; an
// empty value clears the shared Jam.
func normalizeJamURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	if len(raw) > 300 || strings.ContainsAny(raw, " \t\r\n\\<>\"'") {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" || u.Opaque != "" {
		return "", false
	}
	switch strings.ToLower(u.Hostname()) {
	case "open.spotify.com":
		if !spotifyContentPath.MatchString(u.Path) {
			return "", false
		}
	case "spotify.link", "spotify.app.link":
		if !spotifyShortPath.MatchString(u.Path) {
			return "", false
		}
	default:
		return "", false
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), true
}

func jamKey(server, channel string) string {
	return strings.ToLower(server) + "/" + strings.ToLower(channel)
}

// callMu must be held. A Jam ends with its call: no members, no invitation.
func (s *Server) pruneJamsLocked() bool {
	pruned := false
	for key := range s.jams {
		active := false
		for _, member := range s.calls {
			if jamKey(member.server, member.channel) == key {
				active = true
				break
			}
		}
		if !active {
			delete(s.jams, key)
			pruned = true
		}
	}
	return pruned
}

// p.mu must be held. Six changes per 30 seconds per connection.
func (s *Server) setJam(p *peer, raw string) bool {
	link, ok := normalizeJamURL(raw)
	if !ok {
		return write(p.conn, Event{Type: "call.error", Code: "invalid_jam"}) == nil
	}
	if time.Since(p.jamWindow) >= 30*time.Second {
		p.jamWindow = time.Now()
		p.jamCount = 0
	}
	p.jamCount++
	if p.jamCount > 6 {
		return write(p.conn, Event{Type: "call.error", Code: "rate_limited"}) == nil
	}
	s.callMu.Lock()
	member, joined := s.calls[p]
	if !joined || member.server != p.server || member.channel != p.channel {
		s.callMu.Unlock()
		return write(p.conn, Event{Type: "call.error", Code: "not_joined"}) == nil
	}
	key := jamKey(member.server, member.channel)
	current := s.jams[key]
	changed := false
	if link == "" && current != nil {
		delete(s.jams, key)
		changed = true
	} else if link != "" && (current == nil || current.URL != link) {
		s.jams[key] = &CallJam{URL: link, User: member.participant.User, Name: member.participant.Name, Since: time.Now().Unix()}
		changed = true
	}
	if changed {
		s.callVersion++
	}
	s.callMu.Unlock()
	if changed {
		s.callDirty()
	}
	return true
}
