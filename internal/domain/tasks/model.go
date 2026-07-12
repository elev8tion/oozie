package tasks

import "time"

type Task struct {
	ID          int64
	Title       string
	CompletedAt *time.Time
	CreatedAt   time.Time
}

func (t Task) Completed() bool {
	return t.CompletedAt != nil
}
