package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Repo struct {
	database *sql.DB
}

func NewRepo(database *sql.DB) *Repo {
	return &Repo{database: database}
}

func (r *Repo) List(ctx context.Context) ([]Task, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, title, completed_at, created_at
		FROM tasks
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var task Task
		var completedAt sql.NullTime
		if err := rows.Scan(&task.ID, &task.Title, &completedAt, &task.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if completedAt.Valid {
			task.CompletedAt = &completedAt.Time
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *Repo) GetByID(ctx context.Context, id int64) (Task, error) {
	var task Task
	var completedAt sql.NullTime
	err := r.database.QueryRowContext(ctx, `
		SELECT id, title, completed_at, created_at
		FROM tasks
		WHERE id = ?
	`, id).Scan(&task.ID, &task.Title, &completedAt, &task.CreatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("get task: %w", err)
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	return task, nil
}

func (r *Repo) Create(ctx context.Context, title string) (Task, error) {
	result, err := r.database.ExecContext(ctx, `INSERT INTO tasks (title) VALUES (?)`, title)
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Task{}, fmt.Errorf("created task id: %w", err)
	}
	return r.GetByID(ctx, id)
}

func (r *Repo) Complete(ctx context.Context, id int64) (Task, error) {
	_, err := r.database.ExecContext(ctx, `UPDATE tasks SET completed_at = ? WHERE id = ?`, time.Now().UTC(), id)
	if err != nil {
		return Task{}, fmt.Errorf("complete task: %w", err)
	}
	return r.GetByID(ctx, id)
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	if _, err := r.database.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}
