package kanban

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func handleKanbanTaskInit(s *discordgo.Session, logger *slog.Logger, i *discordgo.InteractionCreate) {
	if err := mustGuild(i); err != nil {
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	ctx, ok := mustTaskContext(s, i)
	if !ok {
		// mustTaskContext already responded
		return
	}

	// Load projects
	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, okProj, hint := findProjectByThreadContext(projects, i.GuildID, ctx.ForumID)
	if !okProj {
		respondEphemeral(s, i, "error: "+hint)
		return
	}

	// Ensure tag mapping exists for forum (fetch if needed)
	p, err = ensureForumTagMapping(s, p, ctx.ForumID)
	if err != nil {
		logger.Error("ensure tags failed", "err", err, "slug", p.Slug, "forum", ctx.ForumID)
		respondEphemeral(s, i, "error: failed to resolve forum tags: "+err.Error())
		return
	}

	task := p.Tasks[ctx.ThreadID]
	already := strings.TrimSpace(task.ThreadID) != ""

	task.ThreadID = ctx.ThreadID
	task.ForumID = ctx.ForumID
	if strings.TrimSpace(string(task.Status)) == "" {
		task.Status = TaskToDo
	}

	// Create or update status panel
	msgID, err := ensureStatusPanel(s, p, task)
	if err != nil {
		logger.Error("ensure panel failed", "err", err, "slug", p.Slug, "thread", ctx.ThreadID)
		respondEphemeral(s, i, "error: failed to create status panel: "+err.Error())
		return
	}
	task.StatusMessageID = msgID

	// Force tag to match status (init => ToDo by default)
	if task.Status != TaskToDo {
		task.Status = TaskToDo
		task.AssigneeUserID = ""
		task.DoneDescription = ""
		task.ApprovedByUserID = ""
	}
	if err := applyStatusTagToThread(s, p, ctx.ForumID, ctx.ThreadID, task.Status); err != nil {
		logger.Error("apply tag failed", "err", err, "slug", p.Slug, "thread", ctx.ThreadID, "status", task.Status)
		respondEphemeral(s, i, "error: failed to apply tag: "+err.Error())
		return
	}

	// Update panel now that tag/status is consistent
	if err := upsertStatusPanel(s, p, task); err != nil {
		logger.Error("update panel failed", "err", err, "slug", p.Slug, "thread", ctx.ThreadID)
		respondEphemeral(s, i, "error: failed to update status panel: "+err.Error())
		return
	}

	p.Tasks[ctx.ThreadID] = task
	if err := updateFile(p); err != nil {
		logger.Error("update project file failed", "err", err, "slug", p.Slug)
		respondEphemeral(s, i, "error: init done, but failed to save json: "+err.Error())
		return
	}

	if already {
		respondEphemeral(s, i, "task already initialized ✅ (panel refreshed)")
		return
	}
	respondEphemeral(s, i, "task initialized ✅ (status panel pinned, tag set to ToDo)")
}

func handleKanbanTaskTake(s *discordgo.Session, logger *slog.Logger, i *discordgo.InteractionCreate) {
	if err := mustGuild(i); err != nil {
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	authorID := getAuthorID(i)
	if authorID == "" {
		respondEphemeral(s, i, "error: cannot detect author")
		return
	}

	ctx, ok := mustTaskContext(s, i)
	if !ok {
		return
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, okProj, hint := findProjectByThreadContext(projects, i.GuildID, ctx.ForumID)
	if !okProj {
		respondEphemeral(s, i, "error: "+hint)
		return
	}

	p, err = ensureForumTagMapping(s, p, ctx.ForumID)
	if err != nil {
		respondEphemeral(s, i, "error: failed to resolve forum tags: "+err.Error())
		return
	}

	task, okTask := p.Tasks[ctx.ThreadID]
	if !okTask || strings.TrimSpace(task.ThreadID) == "" {
		respondEphemeral(s, i, "error: task not initialized. Run /kanban task-init in this thread.")
		return
	}

	// Status must be ToDo (and unassigned)
	if task.Status != TaskToDo {
		respondEphemeral(s, i, "not allowed: task status is not ToDo")
		return
	}
	if strings.TrimSpace(task.AssigneeUserID) != "" {
		respondEphemeral(s, i, "not allowed: task already taken")
		return
	}

	task.Status = TaskInProgress
	task.AssigneeUserID = authorID
	task.DoneDescription = ""
	task.ApprovedByUserID = ""

	if err := applyStatusTagToThread(s, p, ctx.ForumID, ctx.ThreadID, task.Status); err != nil {
		logger.Error("apply tag failed", "err", err, "slug", p.Slug, "thread", ctx.ThreadID)
		respondEphemeral(s, i, "error: failed to apply tag: "+err.Error())
		return
	}

	if err := upsertStatusPanel(s, p, task); err != nil {
		logger.Error("update panel failed", "err", err, "slug", p.Slug, "thread", ctx.ThreadID)
		respondEphemeral(s, i, "error: failed to update status panel: "+err.Error())
		return
	}

	p.Tasks[ctx.ThreadID] = task
	if err := updateFile(p); err != nil {
		respondEphemeral(s, i, "error: updated task, but failed to save json: "+err.Error())
		return
	}

	respondEphemeral(s, i, "taken ✅ (status set to InProgress)")
}

func handleKanbanTaskDone(s *discordgo.Session, logger *slog.Logger, i *discordgo.InteractionCreate, description string) {
	if err := mustGuild(i); err != nil {
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	description = strings.TrimSpace(description)
	if description == "" {
		respondEphemeral(s, i, "error: description is required")
		return
	}

	authorID := getAuthorID(i)
	if authorID == "" {
		respondEphemeral(s, i, "error: cannot detect author")
		return
	}

	ctx, ok := mustTaskContext(s, i)
	if !ok {
		return
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, okProj, hint := findProjectByThreadContext(projects, i.GuildID, ctx.ForumID)
	if !okProj {
		respondEphemeral(s, i, "error: "+hint)
		return
	}

	p, err = ensureForumTagMapping(s, p, ctx.ForumID)
	if err != nil {
		respondEphemeral(s, i, "error: failed to resolve forum tags: "+err.Error())
		return
	}

	task, okTask := p.Tasks[ctx.ThreadID]
	if !okTask || strings.TrimSpace(task.ThreadID) == "" {
		respondEphemeral(s, i, "error: task not initialized. Run /kanban task-init in this thread.")
		return
	}

	if task.Status != TaskInProgress {
		respondEphemeral(s, i, "not allowed: task status is not InProgress")
		return
	}

	// Only assignee OR leader can submit for approval
	if !isLeaderForProject(i, authorID, p) && strings.TrimSpace(task.AssigneeUserID) != authorID {
		respondEphemeral(s, i, "not allowed: only assignee or leader can do this")
		return
	}

	task.Status = TaskWaitingForApprove
	task.DoneDescription = description
	task.ApprovedByUserID = ""

	if err := applyStatusTagToThread(s, p, ctx.ForumID, ctx.ThreadID, task.Status); err != nil {
		respondEphemeral(s, i, "error: failed to apply tag: "+err.Error())
		return
	}

	if err := upsertStatusPanel(s, p, task); err != nil {
		respondEphemeral(s, i, "error: failed to update status panel: "+err.Error())
		return
	}

	// Optional: post a visible message in thread so reviewers see it (not ephemeral)
	_, _ = s.ChannelMessageSend(ctx.ThreadID, fmt.Sprintf("🟦 Submitted for approval by <@%s>\n\n%s", authorID, description))

	p.Tasks[ctx.ThreadID] = task
	if err := updateFile(p); err != nil {
		respondEphemeral(s, i, "error: updated task, but failed to save json: "+err.Error())
		return
	}

	respondEphemeral(s, i, "submitted ✅ (status set to WaitingForApprove)")
}

func handleKanbanTaskApprove(s *discordgo.Session, logger *slog.Logger, i *discordgo.InteractionCreate) {
	if err := mustGuild(i); err != nil {
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	authorID := getAuthorID(i)
	if authorID == "" {
		respondEphemeral(s, i, "error: cannot detect author")
		return
	}

	ctx, ok := mustTaskContext(s, i)
	if !ok {
		return
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, okProj, hint := findProjectByThreadContext(projects, i.GuildID, ctx.ForumID)
	if !okProj {
		respondEphemeral(s, i, "error: "+hint)
		return
	}

	// Leader only
	if !isLeaderForProject(i, authorID, p) {
		respondEphemeral(s, i, "not allowed: only project leader can approve")
		return
	}

	p, err = ensureForumTagMapping(s, p, ctx.ForumID)
	if err != nil {
		respondEphemeral(s, i, "error: failed to resolve forum tags: "+err.Error())
		return
	}

	task, okTask := p.Tasks[ctx.ThreadID]
	if !okTask || strings.TrimSpace(task.ThreadID) == "" {
		respondEphemeral(s, i, "error: task not initialized. Run /kanban task-init in this thread.")
		return
	}

	if task.Status != TaskWaitingForApprove {
		respondEphemeral(s, i, "not allowed: task status is not WaitingForApprove")
		return
	}

	task.Status = TaskDone
	task.ApprovedByUserID = authorID

	if err := applyStatusTagToThread(s, p, ctx.ForumID, ctx.ThreadID, task.Status); err != nil {
		respondEphemeral(s, i, "error: failed to apply tag: "+err.Error())
		return
	}

	if err := upsertStatusPanel(s, p, task); err != nil {
		respondEphemeral(s, i, "error: failed to update status panel: "+err.Error())
		return
	}

	_, _ = s.ChannelMessageSend(ctx.ThreadID, fmt.Sprintf("✅ Approved by <@%s> at %s", authorID, time.Now().Format(time.RFC3339)))

	p.Tasks[ctx.ThreadID] = task
	if err := updateFile(p); err != nil {
		respondEphemeral(s, i, "error: updated task, but failed to save json: "+err.Error())
		return
	}

	respondEphemeral(s, i, "approved ✅ (status set to Done)")
}

func handleKanbanTaskRevoke(s *discordgo.Session, logger *slog.Logger, i *discordgo.InteractionCreate) {
	if err := mustGuild(i); err != nil {
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	authorID := getAuthorID(i)
	if authorID == "" {
		respondEphemeral(s, i, "error: cannot detect author")
		return
	}

	ctx, ok := mustTaskContext(s, i)
	if !ok {
		return
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, okProj, hint := findProjectByThreadContext(projects, i.GuildID, ctx.ForumID)
	if !okProj {
		respondEphemeral(s, i, "error: "+hint)
		return
	}

	// Leader only
	if !isLeaderForProject(i, authorID, p) {
		respondEphemeral(s, i, "not allowed: only project leader can revoke")
		return
	}

	p, err = ensureForumTagMapping(s, p, ctx.ForumID)
	if err != nil {
		respondEphemeral(s, i, "error: failed to resolve forum tags: "+err.Error())
		return
	}

	task, okTask := p.Tasks[ctx.ThreadID]
	if !okTask || strings.TrimSpace(task.ThreadID) == "" {
		respondEphemeral(s, i, "error: task not initialized. Run /kanban task-init in this thread.")
		return
	}

	if task.Status != TaskWaitingForApprove {
		respondEphemeral(s, i, "not allowed: task status is not WaitingForApprove")
		return
	}

	task.Status = TaskInProgress
	task.ApprovedByUserID = ""
	task.DoneDescription = "" // keep workflow clean

	if err := applyStatusTagToThread(s, p, ctx.ForumID, ctx.ThreadID, task.Status); err != nil {
		respondEphemeral(s, i, "error: failed to apply tag: "+err.Error())
		return
	}

	if err := upsertStatusPanel(s, p, task); err != nil {
		respondEphemeral(s, i, "error: failed to update status panel: "+err.Error())
		return
	}

	p.Tasks[ctx.ThreadID] = task
	if err := updateFile(p); err != nil {
		respondEphemeral(s, i, "error: updated task, but failed to save json: "+err.Error())
		return
	}

	respondEphemeral(s, i, "revoked ✅ (back to InProgress)")
}

func handleKanbanTaskSurrender(s *discordgo.Session, logger *slog.Logger, i *discordgo.InteractionCreate) {
	if err := mustGuild(i); err != nil {
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	authorID := getAuthorID(i)
	if authorID == "" {
		respondEphemeral(s, i, "error: cannot detect author")
		return
	}

	ctx, ok := mustTaskContext(s, i)
	if !ok {
		return
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, okProj, hint := findProjectByThreadContext(projects, i.GuildID, ctx.ForumID)
	if !okProj {
		respondEphemeral(s, i, "error: "+hint)
		return
	}

	p, err = ensureForumTagMapping(s, p, ctx.ForumID)
	if err != nil {
		respondEphemeral(s, i, "error: failed to resolve forum tags: "+err.Error())
		return
	}

	task, okTask := p.Tasks[ctx.ThreadID]
	if !okTask || strings.TrimSpace(task.ThreadID) == "" {
		respondEphemeral(s, i, "error: task not initialized. Run /kanban task-init in this thread.")
		return
	}

	if task.Status != TaskInProgress {
		respondEphemeral(s, i, "not allowed: task status is not InProgress")
		return
	}

	// Only assignee OR leader
	if !isLeaderForProject(i, authorID, p) && strings.TrimSpace(task.AssigneeUserID) != authorID {
		respondEphemeral(s, i, "not allowed: only assignee or leader can surrender")
		return
	}

	task.Status = TaskToDo
	task.AssigneeUserID = ""
	task.DoneDescription = ""
	task.ApprovedByUserID = ""

	if err := applyStatusTagToThread(s, p, ctx.ForumID, ctx.ThreadID, task.Status); err != nil {
		respondEphemeral(s, i, "error: failed to apply tag: "+err.Error())
		return
	}

	if err := upsertStatusPanel(s, p, task); err != nil {
		respondEphemeral(s, i, "error: failed to update status panel: "+err.Error())
		return
	}

	p.Tasks[ctx.ThreadID] = task
	if err := updateFile(p); err != nil {
		respondEphemeral(s, i, "error: updated task, but failed to save json: "+err.Error())
		return
	}

	respondEphemeral(s, i, "surrendered ✅ (back to ToDo)")
}
