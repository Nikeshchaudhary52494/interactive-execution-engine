package session

type State string

const (
	StateWaiting      State = "WAITING"
	StateCreated      State = "CREATED"
	StateStarting     State = "STARTING"
	StateRunning      State = "RUNNING"
	StateWaitingInput State = "WAITING_FOR_INPUT"
	StateFinished     State = "FINISHED"
	StateTerminated   State = "TERMINATED"
	StateClosed       State = "CLOSED"
)
