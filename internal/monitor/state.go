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

// RoundCheck is one check's part in a round. A check that is not required belongs to the endpoint's address set: Hashi
// connects to whichever address answers, so the set passes when one of them does (07 §2).
type RoundCheck struct {
	Required bool
	Result   Result
}

// Verdict combines one round's results (07 §3): a failing required check, a critical result or an address set with no
// address answering means down; every check passing but with a warning (a certificate close to expiry) or slower than
// the threshold means degraded. latencyMs is the slowest passing check's.
func Verdict(checks []RoundCheck, slowMs, latencyMs int64) string {
	if len(checks) == 0 {
		return StateDown
	}
	set, setOK, degraded := 0, false, false
	for _, c := range checks {
		if c.Result.Critical || (c.Required && !c.Result.OK) {
			return StateDown
		}
		if !c.Required {
			set++
			setOK = setOK || c.Result.OK
		}
		degraded = degraded || (c.Result.OK && c.Result.Degraded)
	}
	switch {
	case set > 0 && !setOK:
		return StateDown
	case degraded, slowMs > 0 && latencyMs > slowMs:
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
