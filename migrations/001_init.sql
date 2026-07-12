CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  display_name TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS organizations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_user_id INTEGER NOT NULL REFERENCES users(id),
  organization_id INTEGER NULL REFERENCES organizations(id),
  name TEXT NOT NULL,
  project_path_display TEXT NOT NULL,
  trusted INTEGER NOT NULL DEFAULT 0,
  archived INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ready',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_projects_updated ON projects(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_projects_archived ON projects(archived);

CREATE TABLE IF NOT EXISTS agent_sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id INTEGER NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK(status IN ('streaming','completed','failed','cancelled','waiting')),
  mode TEXT NOT NULL CHECK(mode IN ('build','plan')),
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMP NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_requests_session ON agent_requests(session_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id INTEGER NOT NULL REFERENCES agent_requests(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK(role IN ('user','assistant','system','tool')),
  status TEXT NOT NULL CHECK(status IN ('loading','completed','error')),
  content TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_pending_questions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  request_id INTEGER NOT NULL REFERENCES agent_requests(id) ON DELETE CASCADE,
  prompt TEXT NOT NULL,
  options_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_permission_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  request_id INTEGER NOT NULL REFERENCES agent_requests(id) ON DELETE CASCADE,
  permission_name TEXT NOT NULL,
  reason TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS attachments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  request_id INTEGER NULL REFERENCES agent_requests(id) ON DELETE SET NULL,
  filename TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  storage_path TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS publish_drafts (
  project_id INTEGER PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  app_name TEXT NOT NULL,
  headline TEXT NOT NULL,
  description TEXT NOT NULL,
  changelog TEXT NOT NULL DEFAULT '',
  publish_target TEXT NOT NULL DEFAULT 'public',
  visibility TEXT NOT NULL DEFAULT 'unlisted',
  screenshot_manifest TEXT NOT NULL DEFAULT '[]',
  organization_id INTEGER NULL REFERENCES organizations(id),
  saved_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS store_apps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NULL REFERENCES projects(id) ON DELETE SET NULL,
  organization_id INTEGER NULL REFERENCES organizations(id),
  name TEXT NOT NULL,
  headline TEXT NOT NULL,
  description TEXT NOT NULL,
  visibility TEXT NOT NULL DEFAULT 'public',
  published_version TEXT NOT NULL DEFAULT '1.0.0',
  last_published_at TIMESTAMP NULL,
  install_count INTEGER NOT NULL DEFAULT 0,
  featured INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS installed_apps (
  store_app_id INTEGER NOT NULL REFERENCES store_apps(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (store_app_id, user_id)
);

CREATE TABLE IF NOT EXISTS publishing_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  store_app_id INTEGER NULL REFERENCES store_apps(id) ON DELETE SET NULL,
  status TEXT NOT NULL CHECK(status IN ('queued','running','succeeded','failed')),
  error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS feedback (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id),
  feedback_type TEXT NOT NULL,
  reason TEXT NOT NULL,
  additional_feedback TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  appearance TEXT NOT NULL DEFAULT 'system',
  style_profile TEXT NOT NULL DEFAULT 'graphite',
  agent_shortcut TEXT NOT NULL DEFAULT 'Cmd+K',
  send_message_shortcut TEXT NOT NULL DEFAULT 'Cmd+Enter',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO users (id, display_name) VALUES (1, 'Solo Builder');
INSERT OR IGNORE INTO organizations (id, name) VALUES (1, 'Personal Workspace');
INSERT OR IGNORE INTO user_settings (user_id) VALUES (1);

INSERT OR IGNORE INTO projects (id, owner_user_id, organization_id, name, project_path_display, trusted, archived, status, created_at, updated_at) VALUES
  (1, 1, 1, 'Launch Notes', '~/Projects/launch-notes', 1, 0, 'building', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
  (2, 1, 1, 'Tiny CRM', '~/Projects/tiny-crm', 1, 0, 'ready', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT OR IGNORE INTO agent_sessions (id, project_id, title) VALUES (1, 1, 'Build session');
INSERT OR IGNORE INTO agent_requests (id, session_id, status, mode, completed_at) VALUES (1, 1, 'completed', 'plan', CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO agent_messages (id, request_id, role, status, content, metadata_json) VALUES
  (1, 1, 'user', 'completed', 'Plan the first version and note publishing risks.', '{}'),
  (2, 1, 'assistant', 'completed', 'Plan ready: create the core workspace, seed a publish draft, and keep store integration stubbed until the UI flow feels right.', '{}');
INSERT OR IGNORE INTO attachments (id, project_id, request_id, filename, mime_type, size_bytes, storage_path) VALUES (1, 1, 1, 'context.md', 'text/markdown', 2048, 'local/context.md');

INSERT OR IGNORE INTO store_apps (id, project_id, organization_id, name, headline, description, visibility, published_version, last_published_at, install_count, featured) VALUES
  (1, NULL, NULL, 'Prompt Vault', 'Save reusable build prompts', 'A compact prompt library for solo builders.', 'public', '1.2.0', CURRENT_TIMESTAMP, 42, 1),
  (2, NULL, 1, 'Draft Board', 'Plan releases before publishing', 'Organization-visible publish planning board.', 'organization', '0.8.1', CURRENT_TIMESTAMP, 7, 1),
  (3, NULL, NULL, 'Bug Lens', 'Review failed sessions quickly', 'Surfaces errors, approvals, and retry notes from agent sessions.', 'public', '1.0.0', CURRENT_TIMESTAMP, 18, 0);

INSERT OR IGNORE INTO installed_apps (store_app_id, user_id) VALUES (1, 1);

INSERT OR IGNORE INTO publish_drafts (project_id, app_name, headline, description, changelog, publish_target, visibility, screenshot_manifest, organization_id) VALUES
  (1, 'Launch Notes', 'A small app for release notes', 'Collect, polish, and publish concise release updates.', '- Initial project shell\n- Store draft flow', 'public', 'unlisted', '[{"name":"dashboard.png"}]', 1);

INSERT OR IGNORE INTO publishing_jobs (id, project_id, store_app_id, status, error_message) VALUES
  (1, 1, 1, 'succeeded', ''),
  (2, 2, NULL, 'failed', 'Missing headline and screenshot manifest.');
