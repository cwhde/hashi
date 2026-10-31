package monitor

// Endpoint states (07 §3). Paused is a flag on the endpoint, shown as a state by the API.
const (
	StateUp       = "up"
	StateDegraded = "degraded"
	StateDown     = "down"
	StateUnknown  = "unknown"
	StatePaused   = "paused"
)

// Transition thresholds (07 §3).
const (
	FailuresToDown = 3
	SuccessesToUp  = 2
)

// RoundCheck is one check's part in a round. Advisory checks (a certificate beside the paths) say something about the
// endpoint without being a path to it: a failing one degrades, a passing one never hides that the paths are dead.
type RoundCheck struct {
	Required bool
	Advisory bool
	Result   Result
}

// Verdict combines one round's results (07 §3): a failing required check or a critical result means down; without
// required checks, every path failing means down; any other failure, a warning, or latency over the threshold means
// degraded. Two paths with one healthy therefore give degraded ("service down" or "edge down", 07 §2).
func Verdict(checks []RoundCheck, slowMs, latencyMs int64) string {
	paths, pathFails, required, degraded := 0, 0, false, false
	for _, c := range checks {
		required = required || c.Required
		degraded = degraded || c.Result.Degraded
		if !c.Advisory {
			paths++
		}
		if c.Result.OK {
			continue
		}
		switch {
		case c.Required, c.Result.Critical:
			return StateDown
		case c.Advisory:
			degraded = true
		default:
			pathFails++
		}
	}
	switch {
	case len(checks) == 0:
		return StateDown
	case !required && paths > 0 && pathFails == paths:
		return StateDown
	case pathFails > 0, degraded, slowMs > 0 && latencyMs > slowMs:
		return StateDegraded
	}
	return StateUp
}

// Machine is an endpoint's state with its round counters.
type Machine struct {
	State string
	Good  int64
	Bad   int64
}

// Next applies a round's verdict. FailuresToDown down verdicts in a row mean down; SuccessesToUp rounds in a row with a
// healthy verdict other than the current state settle on it. Rounds between the thresholds keep the state, so one blip
// neither raises nor clears an alarm.
func (m Machine) Next(verdict string) Machine {
	if verdict == StateDown {
		m.Good, m.Bad = 0, m.Bad+1
		if m.Bad >= FailuresToDown {
			m.State = StateDown
		}
		return m
	}
	m.Bad = 0
	if verdict == m.State {
		m.Good = 0
		return m
	}
	m.Good++
	if m.Good >= SuccessesToUp {
		m.State, m.Good = verdict, 0
	}
	return m
}
