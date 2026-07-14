package projects

import "time"

type User struct {
	ID          int64
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Organization struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

type Project struct {
	ID                 int64
	OwnerUserID        int64
	OrganizationID     *int64
	Name               string
	ProjectPathDisplay string
	Trusted            bool
	Archived           bool
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AgentSession struct {
	ID          int64
	ProjectID   int64
	Title       string
	Model       string
	PiSessionID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type AgentRequest struct {
	ID          int64
	SessionID   int64
	Status      string
	Mode        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

type AgentMessage struct {
	ID           int64
	RequestID    int64
	Role         string
	Status       string
	Content      string
	MetadataJSON string
	CreatedAt    time.Time
}

type PendingQuestion struct {
	ID          int64
	ProjectID   int64
	RequestID   int64
	Prompt      string
	OptionsJSON string
	Options     []string
	Status      string
	RPCID       string
	CreatedAt   time.Time
}

type PermissionRequest struct {
	ID             int64
	ProjectID      int64
	RequestID      int64
	PermissionName string
	Reason         string
	Status         string
	RPCID          string
	CreatedAt      time.Time
}

type PublishDraft struct {
	ProjectID          int64
	AppName            string
	Headline           string
	Description        string
	Changelog          string
	PublishTarget      string
	Visibility         string
	ScreenshotManifest string
	OrganizationID     *int64
	SavedAt            time.Time
}

type StoreApp struct {
	ID               int64
	ProjectID        *int64
	OrganizationID   *int64
	Name             string
	Headline         string
	Description      string
	Visibility       string
	PublishedVersion string
	LastPublishedAt  *time.Time
	InstallCount     int
	Featured         bool
	Installed        bool
	ArtifactPath     string
	CreatedAt        time.Time
}

type PublishingJob struct {
	ID           int64
	ProjectID    int64
	ProjectName  string
	StoreAppID   *int64
	Status       string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Settings struct {
	Appearance   string
	StyleProfile string
}

type Dashboard struct {
	Projects  []Project
	StoreApps []StoreApp
	Jobs      []PublishingJob
}

type ModelOption struct {
	Provider string
	ID       string
	Full     string
}

type AgentPage struct {
	Project    Project
	Session    AgentSession
	Requests   []AgentRequest
	Messages   []AgentMessage
	Question   *PendingQuestion
	Permission *PermissionRequest
	Mode       string
	Error      string
	Models     []ModelOption
	Model      string
	Streaming  bool
}

type ErrValidation struct{ Message string }

func (e ErrValidation) Error() string { return e.Message }
