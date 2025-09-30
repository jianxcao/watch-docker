package composecli

type StackStatus string

var (
	StatusRunning      = StackStatus("running")
	StatusExited       = StackStatus("exited")
	StatusDraft        = StackStatus("draft")
	StatusPartial      = StackStatus("partial")
	StatusCreatedStack = StackStatus("created_stack")
	StatusUnknown      = StackStatus("unknown")
)

// ComposeProject Docker Compose 项目信息
type ComposeProject struct {
	Name         string      `json:"name"`
	ComposeFile  string      `json:"composeFile"`
	Status       StackStatus `json:"status"`
	RunningCount int         `json:"runningCount"`
	ExitedCount  int         `json:"exitedCount"`
	CreatedCount int         `json:"createdCount"`
}
