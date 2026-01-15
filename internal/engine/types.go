package engine

type ExecuteRequest struct {
    Language     string
    Code         string
    TimeLimitMs int64
}

type ExecuteResult struct {
    ExitCode   int
    Stdout     string
    Stderr     string
    DurationMs int64
    TimedOut   bool
}
