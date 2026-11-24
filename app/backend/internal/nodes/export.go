package nodes

import "time"

// AgentBodyLimit is the largest agent request body the console reads.
const AgentBodyLimit = agentBodyLimit

// SetClock replaces the clock behind enrollment expiry, signature skew and
// online status.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SetPollHold sets how long a long poll waits for a configuration change.
func (s *Service) SetPollHold(hold time.Duration) { s.hold = hold }

// PollHold reports how long a long poll waits.
func (s *Service) PollHold() time.Duration { return s.hold }

// SetLimits sets the signed requests allowed per node in each window and
// the enrollment attempts allowed per client address.
func (s *Service) SetLimits(agent, enroll int) {
	s.agentLimit = agent
	s.enrollLimit = enroll
}
