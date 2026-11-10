package alerts

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/tmux"
)

func quiet(q config.QuietHours, now time.Time) bool {
	if q.Start == "" || q.End == "" {
		return false
	}
	loc := time.Local
	if q.Location != "" {
		if l, err := time.LoadLocation(q.Location); err == nil {
			loc = l
		}
	}
	now = now.In(loc)
	startMin, err1 := parseHM(q.Start)
	endMin, err2 := parseHM(q.End)
	if err1 != nil || err2 != nil {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	if startMin == endMin {
		return false
	}
	if startMin < endMin {
		return cur >= startMin && cur < endMin
	}
	return cur >= startMin || cur < endMin
}

func parseHM(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return 0, fmt.Errorf("bad time")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return h*60 + m, nil
}

func marshal(v any) ([]byte, error) { return json.Marshal(v) }

func tmuxFocus(target string) string {
	// target is project/name
	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	sess := tmux.ShadowSession(parts[0])
	return fmt.Sprintf("tmux -L %s select-window -t %s:%s", tmux.SocketName, sess, parts[1])
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
