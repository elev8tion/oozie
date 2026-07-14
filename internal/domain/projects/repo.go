package projects

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

func (r *Repo) ListProjects(ctx context.Context, q, filter string) ([]Project, error) {
	where := []string{"1=1"}
	args := []any{}
	if q != "" {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+q+"%")
	}
	if filter == "archived" {
		where = append(where, "archived = 1")
	} else if filter == "active" || filter == "" {
		where = append(where, "archived = 0")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, owner_user_id, organization_id, name, project_path_display, trusted, archived, status, created_at, updated_at FROM projects WHERE `+strings.Join(where, " AND ")+` ORDER BY updated_at DESC LIMIT 25`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		var org sql.NullInt64
		if err := rows.Scan(&p.ID, &p.OwnerUserID, &org, &p.Name, &p.ProjectPathDisplay, &p.Trusted, &p.Archived, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if org.Valid {
			p.OrganizationID = &org.Int64
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) GetProject(ctx context.Context, id int64) (Project, error) {
	var p Project
	var org sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT id, owner_user_id, organization_id, name, project_path_display, trusted, archived, status, created_at, updated_at FROM projects WHERE id=?`, id).Scan(&p.ID, &p.OwnerUserID, &org, &p.Name, &p.ProjectPathDisplay, &p.Trusted, &p.Archived, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if org.Valid {
		p.OrganizationID = &org.Int64
	}
	return p, err
}

func (r *Repo) CreateProject(ctx context.Context, name, path string, trusted bool) (Project, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO projects (owner_user_id, organization_id, name, project_path_display, trusted, archived, status) VALUES (1,1,?,?,?,?,?)`, name, path, trusted, false, "ready")
	if err != nil {
		return Project{}, err
	}
	id, _ := res.LastInsertId()
	_, _ = r.db.ExecContext(ctx, `INSERT INTO agent_sessions (project_id,title) VALUES (?,?)`, id, "Build session")
	return r.GetProject(ctx, id)
}

func (r *Repo) ArchiveProject(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE projects SET archived=1, status='archived', updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func (r *Repo) GetSession(ctx context.Context, projectID int64) (AgentSession, error) {
	var s AgentSession
	err := r.db.QueryRowContext(ctx, `SELECT id, project_id, title, model, pi_session_id, created_at, updated_at FROM agent_sessions WHERE project_id=? ORDER BY id DESC LIMIT 1`, projectID).Scan(&s.ID, &s.ProjectID, &s.Title, &s.Model, &s.PiSessionID, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		_, e := r.db.ExecContext(ctx, `INSERT INTO agent_sessions (project_id,title) VALUES (?,?)`, projectID, "Build session")
		if e != nil {
			return s, e
		}
		return r.GetSession(ctx, projectID)
	}
	return s, err
}

func (r *Repo) ListMessages(ctx context.Context, sessionID int64) ([]AgentMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT m.id,m.request_id,m.role,m.status,m.content,m.metadata_json,m.created_at FROM agent_messages m JOIN agent_requests ar ON ar.id=m.request_id WHERE ar.session_id=? ORDER BY m.created_at, m.id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentMessage
	for rows.Next() {
		var m AgentMessage
		if err := rows.Scan(&m.ID, &m.RequestID, &m.Role, &m.Status, &m.Content, &m.MetadataJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repo) ListRequests(ctx context.Context, sessionID int64) ([]AgentRequest, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,session_id,status,mode,created_at,updated_at,completed_at FROM agent_requests WHERE session_id=? ORDER BY id DESC LIMIT 10`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentRequest
	for rows.Next() {
		var ar AgentRequest
		if err := rows.Scan(&ar.ID, &ar.SessionID, &ar.Status, &ar.Mode, &ar.CreatedAt, &ar.UpdatedAt, &ar.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, ar)
	}
	return out, rows.Err()
}

func (r *Repo) CreateAgentRequest(ctx context.Context, sessionID int64, mode, message string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO agent_requests (session_id,status,mode) VALUES (?,'streaming',?)`, sessionID, mode)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_messages (request_id,role,status,content,metadata_json) VALUES (?,'user','completed',?,'{}')`, id, message); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (r *Repo) InsertMessage(ctx context.Context, requestID int64, role, status, content string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO agent_messages (request_id,role,status,content,metadata_json) VALUES (?,?,?,?,'{}')`, requestID, role, status, content)
	return err
}

// UpsertPartialAssistant keeps a single 'loading' assistant row per request
// updated with the streaming text-so-far.
func (r *Repo) UpsertPartialAssistant(ctx context.Context, requestID int64, content string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE agent_messages SET content=? WHERE request_id=? AND role='assistant' AND status='loading'`, content, requestID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	return r.InsertMessage(ctx, requestID, "assistant", "loading", content)
}

// FinalizeAssistant replaces the streaming 'loading' row with the final
// message text, or inserts a completed row if streaming never produced one.
func (r *Repo) FinalizeAssistant(ctx context.Context, requestID int64, content string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE agent_messages SET content=?, status='completed' WHERE request_id=? AND role='assistant' AND status='loading'`, content, requestID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	return r.InsertMessage(ctx, requestID, "assistant", "completed", content)
}

func (r *Repo) CompleteRequest(ctx context.Context, id int64, status string) error {
	// Settle any leftover streaming rows so nothing stays 'loading' forever.
	_, _ = r.db.ExecContext(ctx, `UPDATE agent_messages SET status='completed' WHERE request_id=? AND status='loading'`, id)
	_, err := r.db.ExecContext(ctx, `UPDATE agent_requests SET status=?, updated_at=CURRENT_TIMESTAMP, completed_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('streaming','waiting')`, status, id)
	return err
}

func (r *Repo) SetSessionModel(ctx context.Context, sessionID int64, model string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE agent_sessions SET model=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, model, sessionID)
	return err
}

func (r *Repo) SetPiSessionID(ctx context.Context, sessionID int64, piSessionID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE agent_sessions SET pi_session_id=? WHERE id=?`, piSessionID, sessionID)
	return err
}

func (r *Repo) InsertQuestion(ctx context.Context, projectID, requestID int64, rpcID, prompt, optionsJSON string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO agent_pending_questions (project_id,request_id,prompt,options_json,status,rpc_id) VALUES (?,?,?,?,'pending',?)`, projectID, requestID, prompt, optionsJSON, rpcID)
	return err
}

func (r *Repo) InsertPermission(ctx context.Context, projectID, requestID int64, rpcID, name, reason string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO agent_permission_requests (project_id,request_id,permission_name,reason,status,rpc_id) VALUES (?,?,?,?,'pending',?)`, projectID, requestID, name, reason, rpcID)
	return err
}

func (r *Repo) GetQuestion(ctx context.Context, id int64) (PendingQuestion, error) {
	var q PendingQuestion
	err := r.db.QueryRowContext(ctx, `SELECT id,project_id,request_id,prompt,options_json,status,rpc_id,created_at FROM agent_pending_questions WHERE id=?`, id).Scan(&q.ID, &q.ProjectID, &q.RequestID, &q.Prompt, &q.OptionsJSON, &q.Status, &q.RPCID, &q.CreatedAt)
	return q, err
}

func (r *Repo) GetPermission(ctx context.Context, id int64) (PermissionRequest, error) {
	var p PermissionRequest
	err := r.db.QueryRowContext(ctx, `SELECT id,project_id,request_id,permission_name,reason,status,rpc_id,created_at FROM agent_permission_requests WHERE id=?`, id).Scan(&p.ID, &p.ProjectID, &p.RequestID, &p.PermissionName, &p.Reason, &p.Status, &p.RPCID, &p.CreatedAt)
	return p, err
}

func (r *Repo) CancelRequest(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE agent_requests SET status='cancelled', updated_at=CURRENT_TIMESTAMP, completed_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}
func (r *Repo) ResolveQuestion(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE agent_pending_questions SET status=? WHERE id=?`, status, id)
	return err
}
func (r *Repo) ResolvePermission(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE agent_permission_requests SET status=? WHERE id=?`, status, id)
	return err
}
func (r *Repo) PendingQuestion(ctx context.Context, projectID int64) (*PendingQuestion, error) {
	var q PendingQuestion
	err := r.db.QueryRowContext(ctx, `SELECT id,project_id,request_id,prompt,options_json,status,rpc_id,created_at FROM agent_pending_questions WHERE project_id=? AND status='pending' ORDER BY id DESC LIMIT 1`, projectID).Scan(&q.ID, &q.ProjectID, &q.RequestID, &q.Prompt, &q.OptionsJSON, &q.Status, &q.RPCID, &q.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil {
		_ = json.Unmarshal([]byte(q.OptionsJSON), &q.Options)
	}
	return &q, err
}
func (r *Repo) PendingPermission(ctx context.Context, projectID int64) (*PermissionRequest, error) {
	var p PermissionRequest
	err := r.db.QueryRowContext(ctx, `SELECT id,project_id,request_id,permission_name,reason,status,rpc_id,created_at FROM agent_permission_requests WHERE project_id=? AND status='pending' ORDER BY id DESC LIMIT 1`, projectID).Scan(&p.ID, &p.ProjectID, &p.RequestID, &p.PermissionName, &p.Reason, &p.Status, &p.RPCID, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}
func (r *Repo) SaveFeedback(ctx context.Context, projectID int64, typ, reason, extra string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO feedback (project_id,user_id,feedback_type,reason,additional_feedback) VALUES (?,1,?,?,?)`, projectID, typ, reason, extra)
	return err
}

func (r *Repo) GetDraft(ctx context.Context, projectID int64) (PublishDraft, error) {
	var d PublishDraft
	var org sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT project_id,app_name,headline,description,changelog,publish_target,visibility,screenshot_manifest,organization_id,saved_at FROM publish_drafts WHERE project_id=?`, projectID).Scan(&d.ProjectID, &d.AppName, &d.Headline, &d.Description, &d.Changelog, &d.PublishTarget, &d.Visibility, &d.ScreenshotManifest, &org, &d.SavedAt)
	if org.Valid {
		d.OrganizationID = &org.Int64
	}
	return d, err
}
func (r *Repo) SaveDraft(ctx context.Context, d PublishDraft) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO publish_drafts (project_id,app_name,headline,description,changelog,publish_target,visibility,screenshot_manifest,organization_id,saved_at) VALUES (?,?,?,?,?,?,?,?,1,CURRENT_TIMESTAMP) ON CONFLICT(project_id) DO UPDATE SET app_name=excluded.app_name,headline=excluded.headline,description=excluded.description,changelog=excluded.changelog,publish_target=excluded.publish_target,visibility=excluded.visibility,screenshot_manifest=excluded.screenshot_manifest,saved_at=CURRENT_TIMESTAMP`, d.ProjectID, d.AppName, d.Headline, d.Description, d.Changelog, d.PublishTarget, d.Visibility, d.ScreenshotManifest)
	return err
}
func (r *Repo) CreateJob(ctx context.Context, projectID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO publishing_jobs (project_id,status,error_message) VALUES (?,'queued','')`, projectID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
func (r *Repo) SetJobRunning(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE publishing_jobs SET status='running', updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}
func (r *Repo) FinishJob(ctx context.Context, id int64, status, errMsg string, storeAppID *int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE publishing_jobs SET status=?, error_message=?, store_app_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, errMsg, storeAppID, id)
	return err
}
func (r *Repo) UpsertStoreApp(ctx context.Context, projectID int64, d PublishDraft, artifactPath string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM store_apps WHERE project_id=?`, projectID).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := r.db.ExecContext(ctx, `INSERT INTO store_apps (project_id,organization_id,name,headline,description,visibility,published_version,last_published_at,artifact_path) VALUES (?,1,?,?,?,?,'1.0.0',CURRENT_TIMESTAMP,?)`, projectID, d.AppName, d.Headline, d.Description, d.Visibility, artifactPath)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	if err != nil {
		return 0, err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE store_apps SET name=?, headline=?, description=?, visibility=?, last_published_at=CURRENT_TIMESTAMP, artifact_path=? WHERE id=?`, d.AppName, d.Headline, d.Description, d.Visibility, artifactPath, id)
	return id, err
}
func (r *Repo) ListJobs(ctx context.Context, status string) ([]PublishingJob, error) {
	q := `SELECT j.id,j.project_id,p.name,j.store_app_id,j.status,j.error_message,j.created_at,j.updated_at FROM publishing_jobs j JOIN projects p ON p.id=j.project_id`
	args := []any{}
	if status != "" {
		q += ` WHERE j.status=?`
		args = append(args, status)
	}
	q += ` ORDER BY j.updated_at DESC LIMIT 50`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublishingJob
	for rows.Next() {
		var j PublishingJob
		var app sql.NullInt64
		if err := rows.Scan(&j.ID, &j.ProjectID, &j.ProjectName, &app, &j.Status, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		if app.Valid {
			j.StoreAppID = &app.Int64
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *Repo) ListStoreApps(ctx context.Context, q, filter string) ([]StoreApp, error) {
	where := []string{"1=1"}
	args := []any{}
	if q != "" {
		where = append(where, "(name LIKE ? OR headline LIKE ?)")
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	if filter == "featured" {
		where = append(where, "featured=1")
	} else if filter == "public" {
		where = append(where, "visibility='public'")
	} else if filter == "org" {
		where = append(where, "organization_id IS NOT NULL")
	} else if filter == "installed" {
		where = append(where, "EXISTS(SELECT 1 FROM installed_apps i WHERE i.store_app_id=s.id)")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT s.id,s.project_id,s.organization_id,s.name,s.headline,s.description,s.visibility,s.published_version,s.last_published_at,s.install_count,s.featured,s.artifact_path, EXISTS(SELECT 1 FROM installed_apps i WHERE i.store_app_id=s.id),s.created_at FROM store_apps s WHERE `+strings.Join(where, " AND ")+` ORDER BY featured DESC, install_count DESC LIMIT 30`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoreApp
	for rows.Next() {
		s, err := scanStoreApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (r *Repo) GetStoreApp(ctx context.Context, id int64) (StoreApp, error) {
	row := r.db.QueryRowContext(ctx, `SELECT s.id,s.project_id,s.organization_id,s.name,s.headline,s.description,s.visibility,s.published_version,s.last_published_at,s.install_count,s.featured,s.artifact_path, EXISTS(SELECT 1 FROM installed_apps i WHERE i.store_app_id=s.id),s.created_at FROM store_apps s WHERE s.id=?`, id)
	return scanStoreApp(row)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanStoreApp(row rowScanner) (StoreApp, error) {
	var s StoreApp
	var pid, oid sql.NullInt64
	if err := row.Scan(&s.ID, &pid, &oid, &s.Name, &s.Headline, &s.Description, &s.Visibility, &s.PublishedVersion, &s.LastPublishedAt, &s.InstallCount, &s.Featured, &s.ArtifactPath, &s.Installed, &s.CreatedAt); err != nil {
		return StoreApp{}, err
	}
	if pid.Valid {
		s.ProjectID = &pid.Int64
	}
	if oid.Valid {
		s.OrganizationID = &oid.Int64
	}
	return s, nil
}
func (r *Repo) InstallApp(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO installed_apps (store_app_id,user_id) VALUES (?,1)`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		_, _ = r.db.ExecContext(ctx, `UPDATE store_apps SET install_count=install_count+1 WHERE id=?`, id)
	}
	return nil
}
func (r *Repo) InstalledApps(ctx context.Context) ([]StoreApp, error) {
	return r.ListStoreApps(ctx, "", "installed")
}

func (r *Repo) GetSettings(ctx context.Context) (Settings, error) {
	var s Settings
	err := r.db.QueryRowContext(ctx, `SELECT appearance,style_profile FROM user_settings WHERE user_id=1`).Scan(&s.Appearance, &s.StyleProfile)
	return s, err
}
func (r *Repo) SaveSettings(ctx context.Context, s Settings) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_settings SET appearance=?,style_profile=?,updated_at=CURRENT_TIMESTAMP WHERE user_id=1`, s.Appearance, s.StyleProfile)
	return err
}
