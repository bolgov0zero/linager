package agent

import (
	"errors"
	"net"
	"strconv"
	"time"
)

// The server changed its agent port (protocol.TypeServerMove). The agent does not trust the move blindly: the new
// address is only tried first, and after maxMoveTries failed attempts the agent returns to the old one and ignores
// the same port for badPortFor, so a port that does not work (a firewall, a typo) cannot cut the agent off.
const (
	maxMoveTries = 3
	badPortFor   = time.Hour
)

// errMoved ends the current session so that the next one dials the new address.
var errMoved = errors.New("the server moved to another port")

// startMove records the new port. The host is the one in use now; only the port changes. It returns false when the
// request is ignored (the same port, a port that failed lately, a bad value).
func startMove(dir string, st *State, port int, now time.Time) (bool, error) {
	if port < 1 || port > 65535 {
		return false, nil
	}
	host, cur, err := net.SplitHostPort(st.Server)
	if err != nil {
		return false, err
	}
	if cur == strconv.Itoa(port) || st.Next == net.JoinHostPort(host, strconv.Itoa(port)) {
		return false, nil
	}
	if st.BadPort == port && now.Unix() < st.BadUntil {
		return false, nil
	}
	st.Next = net.JoinHostPort(host, strconv.Itoa(port))
	st.NextTries = 0
	return true, saveStateOnly(dir, st)
}

// promoteMove makes the address that worked the main one.
func promoteMove(dir string, st *State) error {
	st.Server, st.Next, st.NextTries, st.BadPort, st.BadUntil = st.Next, "", 0, 0, 0
	return saveStateOnly(dir, st)
}

// failMove counts a failed attempt on the new address; after maxMoveTries it drops the address. It reports whether
// the agent went back to the old one.
func failMove(dir string, st *State, now time.Time) (bool, error) {
	st.NextTries++
	if st.NextTries < maxMoveTries {
		return false, saveStateOnly(dir, st)
	}
	if _, p, err := net.SplitHostPort(st.Next); err == nil {
		st.BadPort, _ = strconv.Atoi(p)
		st.BadUntil = now.Add(badPortFor).Unix()
	}
	st.Next, st.NextTries = "", 0
	return true, saveStateOnly(dir, st)
}
