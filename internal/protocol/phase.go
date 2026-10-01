package protocol

type Phase string

const (
	PhaseInit          Phase = "INIT"
	PhaseDeal          Phase = "DEAL"
	PhaseShareExchange Phase = "SHARE_EXCHANGE"
	PhaseVerify        Phase = "VERIFY"
	PhaseComplaint     Phase = "COMPLAINT"
	PhaseConfirm       Phase = "CONFIRM"
	PhaseFinalize      Phase = "FINALIZE"
	PhaseTimedOut      Phase = "TIMED_OUT"
)
