package kanban

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type ProjectRole string

const (
	Leader ProjectRole = "leader"
	Member ProjectRole = "member"

	dataDir = "kanban-data"
)

// TaskStatus is stored in JSON as a stable machine-readable value.
// Keep it independent from forum tag names (tags are UI, status is data).
type TaskStatus string

const (
	TaskToDo              TaskStatus = "todo"
	TaskInProgress        TaskStatus = "in_progress"
	TaskWaitingForApprove TaskStatus = "waiting_for_approve"
	TaskDone              TaskStatus = "done"
)

// ProjectTask represents one forum thread task.
// The Discord thread is the task container.
type ProjectTask struct {
	ThreadID string `json:"thread_id"`          // forum thread channel ID
	ForumID  string `json:"forum_id,omitempty"` // parent forum channel ID (optional but useful)

	Status TaskStatus `json:"status"`

	// AssigneeUserID is who took it (empty = unassigned).
	AssigneeUserID string `json:"assignee_user_id,omitempty"`

	// StatusMessageID is the bot-owned "Task Status Panel" message id (pinned).
	// Bot edits this message to keep read-only status display consistent.
	StatusMessageID string `json:"status_message_id,omitempty"`

	// DoneDescription is required by /kanban done (your rules).
	DoneDescription string `json:"done_description,omitempty"`

	// ApprovedByUserID is set by /kanban approve.
	ApprovedByUserID string `json:"approved_by_user_id,omitempty"`
}

type Project struct {
	GuildID string `json:"guild_id"`

	Name string `json:"name"`
	Slug string `json:"slug"`

	MemberRoleID string                 `json:"member_role_id"`
	LeaderRoleID string                 `json:"leader_role_id"`
	Members      map[string]ProjectRole `json:"members"`

	CategoryID string `json:"category_id"`

	// ForumChannelIDs stores Discord channel IDs for forum channels under this project category.
	// JSON tag remains "forums" for backward compatibility with already saved files.
	ForumChannelIDs []string `json:"forums"`

	// ForumTagIDs stores Forum tag IDs per forum channel.
	//
	// Structure:
	//   forumID -> (tagName -> tagID)
	ForumTagIDs map[string]map[string]string `json:"forum_tag_ids,omitempty"`

	// Tasks stores tasks by thread ID.
	// Structure:
	//   threadID -> ProjectTask
	Tasks map[string]ProjectTask `json:"tasks,omitempty"`
}

var (
	slugRe   = regexp.MustCompile(`[^a-z0-9-]+`)
	storeMux sync.Mutex
)

func createFile(p Project) error {
	storeMux.Lock()
	defer storeMux.Unlock()

	if err := ensureDataDir(); err != nil {
		return err
	}

	p = normalizeProject(p)

	uniqueSlug, err := findAvailableSlug(p.Slug)
	if err != nil {
		return err
	}
	p.Slug = uniqueSlug

	path := projectPathBySlug(p.Slug)
	return writeProjectAtomic(path, p)
}

func updateFile(p Project) error {
	storeMux.Lock()
	defer storeMux.Unlock()

	if err := ensureDataDir(); err != nil {
		return err
	}

	p = normalizeProject(p)
	if p.Slug == "" {
		return fmt.Errorf("slug required for update")
	}

	path := projectPathBySlug(p.Slug)

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("project not found: %s", p.Slug)
	} else if err != nil {
		return err
	}

	return writeProjectAtomic(path, p)
}

func load_all_files() (map[string]Project, error) {
	storeMux.Lock()
	defer storeMux.Unlock()

	if err := ensureDataDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}

	out := make(map[string]Project, 16)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		low := strings.ToLower(name)

		if !strings.HasSuffix(low, ".json") {
			continue
		}
		if strings.HasSuffix(low, ".tmp") {
			continue
		}

		path := filepath.Join(dataDir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		var p Project
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}

		p = normalizeProject(p)
		if p.Slug == "" {
			p.Slug = strings.TrimSuffix(name, filepath.Ext(name))
		}
		out[p.Slug] = p
	}

	return out, nil
}

func delete_file(p Project) error {
	storeMux.Lock()
	defer storeMux.Unlock()

	if err := ensureDataDir(); err != nil {
		return err
	}

	p = normalizeProject(p)
	if p.Slug == "" {
		return fmt.Errorf("slug required for delete")
	}

	path := projectPathBySlug(p.Slug)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

func ensureDataDir() error {
	return os.MkdirAll(filepath.Clean(dataDir), 0o755)
}

func projectPathBySlug(slug string) string {
	return filepath.Join(dataDir, slug+".json")
}

func normalizeProject(p Project) Project {
	p.Name = strings.TrimSpace(p.Name)
	p.GuildID = strings.TrimSpace(p.GuildID)
	p.CategoryID = strings.TrimSpace(p.CategoryID)
	p.MemberRoleID = strings.TrimSpace(p.MemberRoleID)
	p.LeaderRoleID = strings.TrimSpace(p.LeaderRoleID)

	if p.Slug == "" {
		p.Slug = slugify(p.Name)
	} else {
		p.Slug = slugify(p.Slug)
	}

	if p.Members == nil {
		p.Members = make(map[string]ProjectRole)
	}
	if p.ForumChannelIDs == nil {
		p.ForumChannelIDs = make([]string, 0)
	}
	if p.ForumTagIDs == nil {
		p.ForumTagIDs = make(map[string]map[string]string)
	}
	if p.Tasks == nil {
		p.Tasks = make(map[string]ProjectTask)
	}

	// Clean ForumTagIDs.
	for fid, m := range p.ForumTagIDs {
		tf := strings.TrimSpace(fid)
		if tf == "" || m == nil {
			delete(p.ForumTagIDs, fid)
			continue
		}
		if tf != fid {
			p.ForumTagIDs[tf] = m
			delete(p.ForumTagIDs, fid)
		}
		for name, id := range m {
			tn := strings.TrimSpace(name)
			tid := strings.TrimSpace(id)
			if tn == "" || tid == "" {
				delete(m, name)
				continue
			}
			if tn != name {
				delete(m, name)
				m[tn] = tid
			} else {
				m[tn] = tid
			}
		}
	}

	// Clean Tasks.
	for threadID, t := range p.Tasks {
		tid := strings.TrimSpace(threadID)
		if tid == "" {
			delete(p.Tasks, threadID)
			continue
		}
		if tid != threadID {
			delete(p.Tasks, threadID)
		}

		t.ThreadID = strings.TrimSpace(t.ThreadID)
		if t.ThreadID == "" {
			t.ThreadID = tid
		}
		t.ForumID = strings.TrimSpace(t.ForumID)
		t.AssigneeUserID = strings.TrimSpace(t.AssigneeUserID)
		t.StatusMessageID = strings.TrimSpace(t.StatusMessageID)
		t.DoneDescription = strings.TrimSpace(t.DoneDescription)
		t.ApprovedByUserID = strings.TrimSpace(t.ApprovedByUserID)

		if strings.TrimSpace(string(t.Status)) == "" {
			t.Status = TaskToDo
		}

		p.Tasks[tid] = t
	}

	return p
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	if s == "" {
		return "project"
	}
	return s
}

func findAvailableSlug(base string) (string, error) {
	if ok, err := fileNotExists(projectPathBySlug(base)); err != nil {
		return "", err
	} else if ok {
		return base, nil
	}

	for n := 2; n < 10_000; n++ {
		candidate := base + "-" + strconv.Itoa(n)
		if ok, err := fileNotExists(projectPathBySlug(candidate)); err != nil {
			return "", err
		} else if ok {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("too many projects with slug base: %s", base)
}

func fileNotExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, err
}

func writeProjectAtomic(path string, p Project) error {
	tmp := path + ".tmp"

	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func findProjectByInput(projects map[string]Project, input string) (Project, bool, string) {
	in := strings.TrimSpace(input)
	if in == "" {
		return Project{}, false, "empty input"
	}

	if p, ok := projects[in]; ok {
		return p, true, p.Slug
	}

	inSlug := slugify(in)
	if p, ok := projects[inSlug]; ok {
		return p, true, p.Slug
	}

	var matches []Project
	for _, p := range projects {
		if strings.EqualFold(p.Name, in) || slugify(p.Name) == inSlug {
			matches = append(matches, p)
		}
	}

	if len(matches) == 1 {
		return matches[0], true, matches[0].Slug
	}
	if len(matches) > 1 {
		slugs := make([]string, 0, len(matches))
		for _, m := range matches {
			slugs = append(slugs, m.Slug)
		}
		return Project{}, false, "ambiguous, use slug: " + strings.Join(slugs, ", ")
	}

	return Project{}, false, in
}
