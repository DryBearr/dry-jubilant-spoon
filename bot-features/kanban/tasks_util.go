package kanban

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type taskContext struct {
	ThreadID string
	ForumID  string
}

func getAuthorID(i *discordgo.InteractionCreate) string {
	if i == nil {
		return ""
	}
	if i.Member != nil && i.Member.User != nil {
		return strings.TrimSpace(i.Member.User.ID)
	}
	if i.User != nil {
		return strings.TrimSpace(i.User.ID)
	}
	return ""
}

func mustTaskContext(s *discordgo.Session, i *discordgo.InteractionCreate) (taskContext, bool) {
	threadID := strings.TrimSpace(i.ChannelID)
	if threadID == "" {
		respondEphemeral(s, i, "error: this command must be used inside a thread")
		return taskContext{}, false
	}

	ch, err := getChannelSafe(s, threadID)
	if err != nil || ch == nil {
		respondEphemeral(s, i, "error: failed to read current channel")
		return taskContext{}, false
	}

	// Tasks must be inside a thread (forum post thread).
	if ch.Type != discordgo.ChannelTypeGuildPublicThread &&
		ch.Type != discordgo.ChannelTypeGuildPrivateThread &&
		ch.Type != discordgo.ChannelTypeGuildNewsThread {
		respondEphemeral(s, i, "error: this command must be used inside a forum post thread")
		return taskContext{}, false
	}

	forumID := strings.TrimSpace(ch.ParentID)
	if forumID == "" {
		respondEphemeral(s, i, "error: thread has no parent forum")
		return taskContext{}, false
	}

	return taskContext{
		ThreadID: threadID,
		ForumID:  forumID,
	}, true
}

func getChannelSafe(s *discordgo.Session, channelID string) (*discordgo.Channel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("channelID required")
	}
	if s == nil {
		return nil, fmt.Errorf("session is nil")
	}

	if s.State != nil {
		if ch, err := s.State.Channel(channelID); err == nil && ch != nil {
			return ch, nil
		}
	}
	return s.Channel(channelID)
}

func findProjectByThreadContext(projects map[string]Project, guildID, forumID string) (Project, bool, string) {
	guildID = strings.TrimSpace(guildID)
	forumID = strings.TrimSpace(forumID)
	if guildID == "" {
		return Project{}, false, "guild required"
	}
	if forumID == "" {
		return Project{}, false, "forum required"
	}

	var hits []Project
	for _, p := range projects {
		if strings.TrimSpace(p.GuildID) != guildID {
			continue
		}
		for _, fid := range p.ForumChannelIDs {
			if strings.TrimSpace(fid) == forumID {
				hits = append(hits, p)
				break
			}
		}
	}

	switch len(hits) {
	case 0:
		return Project{}, false, "thread is not under any known project forum (create forum via /kanban create-forum)"
	case 1:
		return hits[0], true, hits[0].Slug
	default:
		// Should never happen unless data broken
		return Project{}, false, "multiple projects match this forum (data conflict)"
	}
}

func ensureStatusPanel(s *discordgo.Session, p Project, task ProjectTask) (string, error) {
	// If already exists, just return it.
	if strings.TrimSpace(task.StatusMessageID) != "" {
		return task.StatusMessageID, nil
	}

	embed := buildStatusEmbed(p, task)

	msg, err := s.ChannelMessageSendEmbed(task.ThreadID, embed)
	if err != nil {
		return "", err
	}
	if msg == nil || strings.TrimSpace(msg.ID) == "" {
		return "", fmt.Errorf("discord returned empty message")
	}

	// Pin it (best effort)
	_ = s.ChannelMessagePin(task.ThreadID, msg.ID)

	return msg.ID, nil
}

func upsertStatusPanel(s *discordgo.Session, p Project, task ProjectTask) error {
	msgID := strings.TrimSpace(task.StatusMessageID)
	if msgID == "" {
		var err error
		msgID, err = ensureStatusPanel(s, p, task)
		if err != nil {
			return err
		}
		task.StatusMessageID = msgID
	}

	embed := buildStatusEmbed(p, task)

	_, err := s.ChannelMessageEditEmbed(task.ThreadID, msgID, embed)
	return err
}

func buildStatusEmbed(p Project, task ProjectTask) *discordgo.MessageEmbed {
	statusText := humanStatus(task.Status)

	assignee := "—"
	if strings.TrimSpace(task.AssigneeUserID) != "" {
		assignee = fmt.Sprintf("<@%s>", task.AssigneeUserID)
	}

	approved := "—"
	if strings.TrimSpace(task.ApprovedByUserID) != "" {
		approved = fmt.Sprintf("<@%s>", task.ApprovedByUserID)
	}

	desc := "—"
	if strings.TrimSpace(task.DoneDescription) != "" {
		desc = task.DoneDescription
	}

	fields := []*discordgo.MessageEmbedField{
		{Name: "Project", Value: fmt.Sprintf("%s (`%s`)", p.Name, p.Slug), Inline: false},
		{Name: "Status", Value: statusText, Inline: true},
		{Name: "Assignee", Value: assignee, Inline: true},
		{Name: "Approved By", Value: approved, Inline: true},
		{Name: "Done Description", Value: desc, Inline: false},
	}

	return &discordgo.MessageEmbed{
		Title:       "Task Status Panel",
		Description: "Read-only panel. Use /kanban task-* commands to update.",
		Fields:      fields,
	}
}

func humanStatus(s TaskStatus) string {
	switch s {
	case TaskToDo:
		return "🟥 ToDo"
	case TaskInProgress:
		return "🟨 InProgress"
	case TaskWaitingForApprove:
		return "🟦 WaitingForApprove"
	case TaskDone:
		return "🟩 Done"
	default:
		return string(s)
	}
}
