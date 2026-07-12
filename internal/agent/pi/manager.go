package pi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Sink receives agent activity so the caller can persist it. Calls arrive
// from the process reader goroutine, never concurrently for one project.
type Sink interface {
	AssistantMessage(projectID, requestID int64, content string)
	ToolMessage(projectID, requestID int64, content string)
	RequestSettled(projectID, requestID int64, status string)
	Question(projectID, requestID int64, rpcID, prompt, optionsJSON string)
	Permission(projectID, requestID int64, rpcID, name, reason string)
	AgentError(projectID, requestID int64, message string)
}

// Manager owns one pi RPC subprocess per project.
type Manager struct {
	mu      sync.Mutex
	procs   map[int64]*proc
	sink    Sink
	catalog Catalog
	binary  string
}

func NewManager(catalog Catalog, sink Sink) *Manager {
	binary := os.Getenv("PI_BIN")
	if binary == "" {
		binary = "pi"
	}
	return &Manager{procs: map[int64]*proc{}, sink: sink, catalog: catalog, binary: binary}
}

// StartOptions describe how to launch pi for a project.
type StartOptions struct {
	ProjectID    int64
	Workdir      string
	Model        string // "provider/id", may be empty for pi's default
	PiSessionID  string
	SystemPrompt string
	Trusted      bool
}

// Prompt ensures a pi process is running for the project and sends the
// message. Events stream back through the Sink asynchronously.
func (m *Manager) Prompt(opts StartOptions, requestID int64, message string) error {
	p, err := m.ensure(opts)
	if err != nil {
		return err
	}
	return p.prompt(requestID, message, opts.Model)
}

func (m *Manager) Abort(projectID int64) error {
	if p := m.get(projectID); p != nil {
		return p.send(map[string]any{"type": "abort"})
	}
	return nil
}

// SetModel switches the model on a running process. If no process is
// running, this is a no-op: the model is applied at next launch.
func (m *Manager) SetModel(projectID int64, model string) error {
	p := m.get(projectID)
	if p == nil {
		return nil
	}
	return p.setModel(model)
}

func (m *Manager) RespondValue(projectID int64, rpcID, value string) error {
	if p := m.get(projectID); p != nil {
		return p.send(map[string]any{"type": "extension_ui_response", "id": rpcID, "value": value})
	}
	return fmt.Errorf("agent is not running")
}

func (m *Manager) RespondConfirm(projectID int64, rpcID string, confirmed bool) error {
	if p := m.get(projectID); p != nil {
		return p.send(map[string]any{"type": "extension_ui_response", "id": rpcID, "confirmed": confirmed})
	}
	return fmt.Errorf("agent is not running")
}

func (m *Manager) RespondCancel(projectID int64, rpcID string) error {
	if p := m.get(projectID); p != nil {
		return p.send(map[string]any{"type": "extension_ui_response", "id": rpcID, "cancelled": true})
	}
	return fmt.Errorf("agent is not running")
}

// Streaming reports whether the project's agent is currently working.
func (m *Manager) Streaming(projectID int64) bool {
	if p := m.get(projectID); p != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.streaming
	}
	return false
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, p := range m.procs {
		p.stop()
		delete(m.procs, id)
	}
}

func (m *Manager) get(projectID int64) *proc {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.procs[projectID]
}

func (m *Manager) ensure(opts StartOptions) (*proc, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[opts.ProjectID]; ok && p.alive() {
		return p, nil
	}

	args := []string{"--mode", "rpc"}
	if opts.PiSessionID != "" {
		args = append(args, "--session-id", opts.PiSessionID)
	}
	if opt, ok := splitModel(opts.Model); ok {
		args = append(args, "--provider", opt.Provider, "--model", opt.ID)
	}
	if m.catalog.ThinkingLevel != "" {
		args = append(args, "--thinking", m.catalog.ThinkingLevel)
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}
	if opts.Trusted {
		args = append(args, "--approve")
	} else {
		args = append(args, "--no-approve")
	}

	cmd := exec.Command(m.binary, args...)
	cmd.Dir = opts.Workdir
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pi (%s): %w", m.binary, err)
	}

	p := &proc{
		projectID: opts.ProjectID,
		cmd:       cmd,
		stdin:     stdin,
		sink:      m.sink,
		model:     opts.Model,
		done:      make(chan struct{}),
	}
	m.procs[opts.ProjectID] = p
	go p.readLoop(stdout)
	return p, nil
}

type proc struct {
	projectID int64
	cmd       *exec.Cmd
	sink      Sink

	mu               sync.Mutex
	stdin            io.WriteCloser
	model            string
	streaming        bool
	hadError         bool
	currentRequestID int64
	done             chan struct{}
	exited           bool
}

func (p *proc) alive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.exited
}

func (p *proc) stop() {
	_ = p.send(map[string]any{"type": "abort"})
	p.mu.Lock()
	stdin := p.stdin
	p.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

func (p *proc) send(cmd map[string]any) error {
	body, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.exited {
		return fmt.Errorf("agent process has exited")
	}
	_, err = p.stdin.Write(append(body, '\n'))
	return err
}

func (p *proc) setModel(model string) error {
	opt, ok := splitModel(model)
	if !ok {
		return fmt.Errorf("invalid model %q", model)
	}
	p.mu.Lock()
	same := p.model == model
	p.mu.Unlock()
	if same {
		return nil
	}
	if err := p.send(map[string]any{"type": "set_model", "provider": opt.Provider, "modelId": opt.ID}); err != nil {
		return err
	}
	p.mu.Lock()
	p.model = model
	p.mu.Unlock()
	return nil
}

func (p *proc) prompt(requestID int64, message, model string) error {
	if model != "" {
		if err := p.setModel(model); err != nil {
			return err
		}
	}
	p.mu.Lock()
	p.currentRequestID = requestID
	busy := p.streaming
	p.mu.Unlock()

	cmd := map[string]any{"type": "prompt", "message": message}
	if busy {
		cmd["streamingBehavior"] = "steer"
	}
	return p.send(cmd)
}

// event is a loosely-typed view over pi's RPC event stream.
type event struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Success *bool  `json:"success"`
	Error   string `json:"error"`

	// Message is an object for message_* events but a plain string for
	// extension_ui_request, so it stays raw and is decoded per event type.
	Message json.RawMessage `json:"message"`

	ToolName string         `json:"toolName"`
	Args     map[string]any `json:"args"`
	IsError  bool           `json:"isError"`

	// extension_ui_request payloads are decoded separately in
	// handleUIRequest because their "message" field is a string and
	// would conflict with Message above.
}

func (p *proc) readLoop(stdout io.Reader) {
	reader := bufio.NewReaderSize(stdout, 1024*1024)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line != "" {
			p.handleLine(line)
		}
		if err != nil {
			break
		}
	}
	_ = p.cmd.Wait()

	p.mu.Lock()
	p.exited = true
	requestID := p.currentRequestID
	wasStreaming := p.streaming
	p.streaming = false
	p.currentRequestID = 0
	close(p.done)
	p.mu.Unlock()

	if wasStreaming && requestID != 0 {
		p.sink.AgentError(p.projectID, requestID, "The pi agent process exited unexpectedly.")
		p.sink.RequestSettled(p.projectID, requestID, "failed")
	}
	log.Printf("pi process for project %d exited", p.projectID)
}

func (p *proc) handleLine(line string) {
	var ev event
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		log.Printf("pi project %d: unparseable event: %.200s", p.projectID, line)
		return
	}

	p.mu.Lock()
	requestID := p.currentRequestID
	p.mu.Unlock()

	switch ev.Type {
	case "agent_start":
		p.mu.Lock()
		p.streaming = true
		p.mu.Unlock()

	case "message_end":
		var msg struct {
			Role         string          `json:"role"`
			Content      json.RawMessage `json:"content"`
			StopReason   string          `json:"stopReason"`
			ErrorMessage string          `json:"errorMessage"`
		}
		if len(ev.Message) > 0 && json.Unmarshal(ev.Message, &msg) == nil && msg.Role == "assistant" && requestID != 0 {
			if text := extractText(msg.Content); text != "" {
				p.sink.AssistantMessage(p.projectID, requestID, text)
			}
			if msg.StopReason == "error" || msg.StopReason == "aborted" {
				p.mu.Lock()
				p.hadError = true
				p.mu.Unlock()
				detail := msg.ErrorMessage
				if detail == "" {
					detail = "The model run was " + msg.StopReason + "."
				}
				p.sink.AgentError(p.projectID, requestID, detail)
			}
		}

	case "tool_execution_end":
		if requestID != 0 {
			p.sink.ToolMessage(p.projectID, requestID, describeTool(ev.ToolName, ev.Args, ev.IsError))
		}

	case "agent_settled":
		p.mu.Lock()
		p.streaming = false
		p.currentRequestID = 0
		status := "completed"
		if p.hadError {
			status = "failed"
			p.hadError = false
		}
		p.mu.Unlock()
		if requestID != 0 {
			p.sink.RequestSettled(p.projectID, requestID, status)
		}

	case "extension_ui_request":
		p.handleUIRequest(line, requestID)

	case "response":
		if ev.Success != nil && !*ev.Success && ev.Command == "prompt" && requestID != 0 {
			p.sink.AgentError(p.projectID, requestID, "pi rejected the prompt: "+ev.Error)
			p.sink.RequestSettled(p.projectID, requestID, "failed")
			p.mu.Lock()
			p.currentRequestID = 0
			p.mu.Unlock()
		}
	}
}

func (p *proc) handleUIRequest(line string, requestID int64) {
	var req struct {
		ID      string   `json:"id"`
		Method  string   `json:"method"`
		Title   string   `json:"title"`
		Message string   `json:"message"`
		Options []string `json:"options"`
	}
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return
	}
	switch req.Method {
	case "select", "input", "editor":
		options := "[]"
		if len(req.Options) > 0 {
			if b, err := json.Marshal(req.Options); err == nil {
				options = string(b)
			}
		}
		p.sink.Question(p.projectID, requestID, req.ID, req.Title, options)
	case "confirm":
		p.sink.Permission(p.projectID, requestID, req.ID, req.Title, req.Message)
	default:
		// notify/setStatus/setWidget/etc. are fire-and-forget; ignore.
	}
}

// extractText joins the text blocks of an assistant message. Content is
// either a plain string or an array of typed blocks.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

func describeTool(name string, args map[string]any, isError bool) string {
	detail := ""
	for _, key := range []string{"command", "path", "file_path", "pattern", "url"} {
		if v, ok := args[key].(string); ok && v != "" {
			detail = v
			break
		}
	}
	if len(detail) > 120 {
		detail = detail[:117] + "…"
	}
	status := "ok"
	if isError {
		status = "error"
	}
	if detail == "" {
		return fmt.Sprintf("%s (%s)", name, status)
	}
	return fmt.Sprintf("%s: %s (%s)", name, detail, status)
}
