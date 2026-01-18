package kanban

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func handleInteraction(s *discordgo.Session, logger *slog.Logger, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	if data.Name != commandKanban {
		return
	}

	if len(data.Options) == 0 {
		respondEphemeral(s, i, "use: /kanban create|delete|add-member|remove-member ...")
		return
	}

	sub := data.Options[0]

	switch sub.Name {
	case "create":
		handleKanbanCreate(s, logger, i, sub)
	case "delete":
		handleKanbanDelete(s, logger, i, sub)
	case "create-forum":
		handleKanbanCreateForum(s, logger, i, sub)
	case "delete-forum":
		handleKanbanDeleteForum(s, logger, i, sub)
	case "add-member":
		handleKanbanAddMember(s, logger, i, sub)
	case "remove-member":
		handleKanbanRemoveMember(s, logger, i, sub)
	// ---------------------------
	// Tasks (thread-based)
	// ---------------------------
	case "task-init":
		handleKanbanTaskInit(s, logger, i)
	case "task-take":
		handleKanbanTaskTake(s, logger, i)
	case "task-done":
		desc := strings.TrimSpace(getSubOptionString(sub, "description"))
		handleKanbanTaskDone(s, logger, i, desc)
	case "task-approve":
		handleKanbanTaskApprove(s, logger, i)
	case "task-revoke":
		handleKanbanTaskRevoke(s, logger, i)
	case "task-surrender":
		handleKanbanTaskSurrender(s, logger, i)

	default:
		respondEphemeral(s, i, "unknown subcommand: "+sub.Name)
	}
}

func handleKanbanAddMember(
	s *discordgo.Session,
	logger *slog.Logger,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) {
	if strings.TrimSpace(i.GuildID) == "" {
		respondEphemeral(s, i, "error: guild required")
		return
	}

	targetProject := strings.TrimSpace(getSubOptionString(sub, "project"))
	if targetProject == "" {
		respondEphemeral(s, i, "project is required (slug or name)")
		return
	}

	targetUserID := strings.TrimSpace(getSubOptionUserID(sub, "user"))
	if targetUserID == "" {
		respondEphemeral(s, i, "user is required")
		return
	}

	// Identify command author (for authorization).
	var authorID string
	if i.Member != nil && i.Member.User != nil {
		authorID = strings.TrimSpace(i.Member.User.ID)
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, found, hint := findProjectByInput(projects, targetProject)
	if !found {
		respondEphemeral(s, i, "project not found: "+hint)
		return
	}

	// Authorization: only leader can add/remove.
	if !isLeaderForProject(i, authorID, p) {
		respondEphemeral(s, i, "not allowed: only project leader can add members")
		return
	}

	if strings.TrimSpace(p.MemberRoleID) == "" {
		respondEphemeral(s, i, "error: project has no member role id saved")
		return
	}

	// 1) Grant member role
	if err := s.GuildMemberRoleAdd(i.GuildID, targetUserID, p.MemberRoleID); err != nil {
		logger.Error("assign member role failed", "err", err, "user", targetUserID, "guild", i.GuildID, "role", p.MemberRoleID, "slug", p.Slug)
		respondEphemeral(s, i, "error: failed to assign member role: "+err.Error())
		return
	}

	// 2) Update JSON membership map
	if p.Members == nil {
		p.Members = map[string]ProjectRole{}
	}
	// If already leader, keep leader.
	if p.Members[targetUserID] != Leader {
		p.Members[targetUserID] = Member
	}

	if err := updateFile(p); err != nil {
		logger.Error("update project file failed", "err", err, "slug", p.Slug, "guild", i.GuildID)
		respondEphemeral(s, i, "member role granted, but failed to update json: "+err.Error())
		return
	}

	respondEphemeral(s, i, fmt.Sprintf(
		"added <@%s> to project **%s** (slug: `%s`)",
		targetUserID, p.Name, p.Slug,
	))
}

func handleKanbanRemoveMember(
	s *discordgo.Session,
	logger *slog.Logger,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) {
	if strings.TrimSpace(i.GuildID) == "" {
		respondEphemeral(s, i, "error: guild required")
		return
	}

	targetProject := strings.TrimSpace(getSubOptionString(sub, "project"))
	if targetProject == "" {
		respondEphemeral(s, i, "project is required (slug or name)")
		return
	}

	targetUserID := strings.TrimSpace(getSubOptionUserID(sub, "user"))
	if targetUserID == "" {
		respondEphemeral(s, i, "user is required")
		return
	}

	// Identify command author (for authorization).
	var authorID string
	if i.Member != nil && i.Member.User != nil {
		authorID = strings.TrimSpace(i.Member.User.ID)
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, found, hint := findProjectByInput(projects, targetProject)
	if !found {
		respondEphemeral(s, i, "project not found: "+hint)
		return
	}

	// Authorization: only leader can add/remove.
	if !isLeaderForProject(i, authorID, p) {
		respondEphemeral(s, i, "not allowed: only project leader can remove members")
		return
	}

	var warnings []string

	// 1) Remove member role (if exists)
	if rid := strings.TrimSpace(p.MemberRoleID); rid != "" {
		if err := s.GuildMemberRoleRemove(i.GuildID, targetUserID, rid); err != nil {
			logger.Error("remove member role failed", "err", err, "user", targetUserID, "guild", i.GuildID, "role", rid, "slug", p.Slug)
			warnings = append(warnings, "memberRoleRemove")
		}
	}

	// 2) Remove leader role too (in case user was leader)
	if rid := strings.TrimSpace(p.LeaderRoleID); rid != "" {
		if err := s.GuildMemberRoleRemove(i.GuildID, targetUserID, rid); err != nil {
			logger.Error("remove leader role failed", "err", err, "user", targetUserID, "guild", i.GuildID, "role", rid, "slug", p.Slug)
			warnings = append(warnings, "leaderRoleRemove")
		}
	}

	// 3) Update JSON membership map
	if p.Members != nil {
		delete(p.Members, targetUserID)
	}

	if err := updateFile(p); err != nil {
		logger.Error("update project file failed", "err", err, "slug", p.Slug, "guild", i.GuildID)
		respondEphemeral(s, i, "roles removed, but failed to update json: "+err.Error())
		return
	}

	if len(warnings) == 0 {
		respondEphemeral(s, i, fmt.Sprintf(
			"removed <@%s> from project **%s** (slug: `%s`)",
			targetUserID, p.Name, p.Slug,
		))
		return
	}

	respondEphemeral(s, i, fmt.Sprintf(
		"removed <@%s> from project **%s** (slug: `%s`) with warnings: %s",
		targetUserID, p.Name, p.Slug, strings.Join(warnings, ", "),
	))
}

func handleKanbanCreate(
	s *discordgo.Session,
	logger *slog.Logger,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) {
	projectName := strings.TrimSpace(getSubOptionString(sub, "project"))
	if projectName == "" {
		respondEphemeral(s, i, "project name is required")
		return
	}

	// Identify command author (default project leader).
	var authorID string
	if i.Member != nil && i.Member.User != nil {
		authorID = strings.TrimSpace(i.Member.User.ID)
	}

	// Compute final unique slug BEFORE creating roles/file so role names and JSON slug match.
	baseSlug := slugify(projectName)
	uniqueSlug, err := findAvailableSlug(baseSlug)
	if err != nil {
		logger.Error("find slug failed", "err", err, "project", projectName, "guild", i.GuildID)
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	// 1) Create/reuse roles for this project slug FIRST.
	memberRoleID, leaderRoleID, err := ensureProjectRoles(s, i.GuildID, uniqueSlug)
	if err != nil {
		logger.Error("create roles failed", "err", err, "project", projectName, "guild", i.GuildID, "slug", uniqueSlug)
		respondEphemeral(s, i, "error: failed to create roles: "+err.Error())
		return
	}

	// Assign leader role to the author.
	if authorID != "" && strings.TrimSpace(leaderRoleID) != "" {
		if err := s.GuildMemberRoleAdd(i.GuildID, authorID, leaderRoleID); err != nil {
			logger.Error("assign leader role failed", "err", err, "user", authorID, "guild", i.GuildID, "role", leaderRoleID)
			// not fatal
		}
	}

	// 2) Create PRIVATE category (only member/leader roles can view).
	categoryID, err := createProjectCategory(
		s,
		i.GuildID,
		projectName,
		true, // private
		memberRoleID,
		leaderRoleID,
	)
	if err != nil {
		logger.Error("create category failed", "err", err, "project", projectName, "guild", i.GuildID)
		respondEphemeral(s, i, "error: "+err.Error())
		return
	}

	members := map[string]ProjectRole{}
	if authorID != "" {
		members[authorID] = Leader
	}

	p := Project{
		GuildID: i.GuildID,
		Name:    projectName,
		Slug:    uniqueSlug,

		Members:         members,
		ForumChannelIDs: []string{},

		MemberRoleID: memberRoleID,
		LeaderRoleID: leaderRoleID,

		CategoryID: categoryID,
	}

	if err := createFile(p); err != nil {
		logger.Error("create project file failed", "err", err, "project", projectName, "guild", i.GuildID)
		respondEphemeral(s, i, "created roles/category, but failed to save project json")
		return
	}

	respondEphemeral(s, i, fmt.Sprintf(
		"created roles **%s-member**/**%s-leader**, private category **%s**",
		uniqueSlug, uniqueSlug, projectName,
	))
}

func handleKanbanDelete(
	s *discordgo.Session,
	logger *slog.Logger,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) {
	if strings.TrimSpace(i.GuildID) == "" {
		respondEphemeral(s, i, "error: guild required")
		return
	}

	target := strings.TrimSpace(getSubOptionString(sub, "project"))
	if target == "" {
		respondEphemeral(s, i, "project is required (slug or name)")
		return
	}

	// Identify command author (for authorization).
	var authorID string
	if i.Member != nil && i.Member.User != nil {
		authorID = strings.TrimSpace(i.Member.User.ID)
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, found, hint := findProjectByInput(projects, target)
	if !found {
		respondEphemeral(s, i, "project not found: "+hint)
		return
	}

	// Authorization: only leader can delete.
	if !isLeaderForProject(i, authorID, p) {
		respondEphemeral(s, i, "not allowed: only project leader can delete this project")
		return
	}

	var warnings []string

	// Delete forums first.
	for _, fid := range p.ForumChannelIDs {
		fid = strings.TrimSpace(fid)
		if fid == "" {
			continue
		}
		if _, err := s.ChannelDelete(fid); err != nil {
			logger.Error("delete forum failed", "err", err, "forum", fid, "slug", p.Slug, "guild", i.GuildID)
			warnings = append(warnings, "forum:"+fid)
		}
	}

	// Delete category.
	if cid := strings.TrimSpace(p.CategoryID); cid != "" {
		if _, err := s.ChannelDelete(cid); err != nil {
			logger.Error("delete category failed", "err", err, "category", cid, "slug", p.Slug, "guild", i.GuildID)
			warnings = append(warnings, "category:"+cid)
		}
	}

	// Delete roles.
	if rid := strings.TrimSpace(p.MemberRoleID); rid != "" {
		if err := s.GuildRoleDelete(i.GuildID, rid); err != nil {
			logger.Error("delete member role failed", "err", err, "role", rid, "slug", p.Slug, "guild", i.GuildID)
			warnings = append(warnings, "memberRole:"+rid)
		}
	}
	if rid := strings.TrimSpace(p.LeaderRoleID); rid != "" {
		if err := s.GuildRoleDelete(i.GuildID, rid); err != nil {
			logger.Error("delete leader role failed", "err", err, "role", rid, "slug", p.Slug, "guild", i.GuildID)
			warnings = append(warnings, "leaderRole:"+rid)
		}
	}

	// Delete JSON file last.
	if err := delete_file(Project{Slug: p.Slug}); err != nil {
		logger.Error("delete project file failed", "err", err, "slug", p.Slug, "guild", i.GuildID)
		respondEphemeral(s, i, "deleted discord resources, but failed to delete json: "+err.Error())
		return
	}

	if len(warnings) == 0 {
		respondEphemeral(s, i, fmt.Sprintf("deleted project **%s** (slug: `%s`)", p.Name, p.Slug))
		return
	}

	respondEphemeral(s, i, fmt.Sprintf(
		"deleted project **%s** (slug: `%s`) with warnings: %s",
		p.Name, p.Slug, strings.Join(warnings, ", "),
	))
}

// handleKanbanCreateForum creates a new forum channel under the project's category,
// stores its ID in the project JSON, and (optionally) stores tag IDs if available.
func handleKanbanCreateForum(
	s *discordgo.Session,
	logger *slog.Logger,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) {
	if strings.TrimSpace(i.GuildID) == "" {
		respondEphemeral(s, i, "error: guild required")
		return
	}

	targetProject := strings.TrimSpace(getSubOptionString(sub, "project"))
	if targetProject == "" {
		respondEphemeral(s, i, "project is required (slug or name)")
		return
	}

	forumName := strings.TrimSpace(getSubOptionString(sub, "name"))
	if forumName == "" {
		forumName = "general"
	}

	// Identify command author (for authorization).
	var authorID string
	if i.Member != nil && i.Member.User != nil {
		authorID = strings.TrimSpace(i.Member.User.ID)
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, found, hint := findProjectByInput(projects, targetProject)
	if !found {
		respondEphemeral(s, i, "project not found: "+hint)
		return
	}

	// Authorization: only leader can create forums.
	if !isLeaderForProject(i, authorID, p) {
		respondEphemeral(s, i, "not allowed: only project leader can create forums")
		return
	}

	if strings.TrimSpace(p.CategoryID) == "" {
		respondEphemeral(s, i, "error: project has no category id saved")
		return
	}

	// Create forum (and tags, if your discordgo supports it).
	forumID, tagIDs, forumErr := createProjectForumWithKanbanTags(s, i.GuildID, p.CategoryID, forumName)
	if forumErr != nil {
		logger.Error("create forum failed", "err", forumErr, "guild", i.GuildID, "slug", p.Slug, "category", p.CategoryID)
		respondEphemeral(s, i, "error: failed to create forum: "+forumErr.Error())
		return
	}

	// Update project JSON (avoid duplicates).
	if !containsString(p.ForumChannelIDs, forumID) {
		p.ForumChannelIDs = append(p.ForumChannelIDs, forumID)
	}
	if p.ForumTagIDs == nil {
		p.ForumTagIDs = make(map[string]map[string]string)
	}
	if len(tagIDs) > 0 {
		p.ForumTagIDs[forumID] = tagIDs
	}

	if err := updateFile(p); err != nil {
		logger.Error("update project file failed", "err", err, "guild", i.GuildID, "slug", p.Slug, "forum", forumID)
		respondEphemeral(s, i, "forum created, but failed to update json: "+err.Error())
		return
	}

	respondEphemeral(s, i, fmt.Sprintf(
		"created forum **%s** for project **%s** (slug: `%s`)",
		forumName, p.Name, p.Slug,
	))
}

// handleKanbanDeleteForum deletes a forum channel (by ID or name) under the project
// and removes it from the project JSON.
func handleKanbanDeleteForum(
	s *discordgo.Session,
	logger *slog.Logger,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) {
	if strings.TrimSpace(i.GuildID) == "" {
		respondEphemeral(s, i, "error: guild required")
		return
	}

	targetProject := strings.TrimSpace(getSubOptionString(sub, "project"))
	if targetProject == "" {
		respondEphemeral(s, i, "project is required (slug or name)")
		return
	}

	forumInput := strings.TrimSpace(getSubOptionString(sub, "forum"))
	if forumInput == "" {
		respondEphemeral(s, i, "forum is required (forum channel id or forum name)")
		return
	}

	// Identify command author (for authorization).
	var authorID string
	if i.Member != nil && i.Member.User != nil {
		authorID = strings.TrimSpace(i.Member.User.ID)
	}

	projects, err := load_all_files()
	if err != nil {
		logger.Error("load projects failed", "err", err, "guild", i.GuildID)
		respondEphemeral(s, i, "error: failed to load projects: "+err.Error())
		return
	}

	p, found, hint := findProjectByInput(projects, targetProject)
	if !found {
		respondEphemeral(s, i, "project not found: "+hint)
		return
	}

	// Authorization: only leader can delete forums.
	if !isLeaderForProject(i, authorID, p) {
		respondEphemeral(s, i, "not allowed: only project leader can delete forums")
		return
	}

	forumID, resolveErr := resolveForumIDFromProject(s, p, forumInput)
	if resolveErr != nil {
		respondEphemeral(s, i, "error: "+resolveErr.Error())
		return
	}

	// Delete the forum channel in Discord.
	if _, err := s.ChannelDelete(forumID); err != nil {
		logger.Error("delete forum failed", "err", err, "guild", i.GuildID, "slug", p.Slug, "forum", forumID)
		respondEphemeral(s, i, "error: failed to delete forum: "+err.Error())
		return
	}

	// Remove from JSON lists/maps.
	p.ForumChannelIDs = removeString(p.ForumChannelIDs, forumID)
	if p.ForumTagIDs != nil {
		delete(p.ForumTagIDs, forumID)
	}

	if err := updateFile(p); err != nil {
		logger.Error("update project file failed", "err", err, "guild", i.GuildID, "slug", p.Slug)
		respondEphemeral(s, i, "forum deleted, but failed to update json: "+err.Error())
		return
	}

	respondEphemeral(s, i, fmt.Sprintf(
		"deleted forum (`%s`) from project **%s** (slug: `%s`)",
		forumID, p.Name, p.Slug,
	))
}

// resolveForumIDFromProject resolves a forum ID from user input that can be:
// - a channel ID (snowflake) existing in p.ForumChannelIDs
// - a forum name matching one of the stored forum channels
func resolveForumIDFromProject(s *discordgo.Session, p Project, input string) (string, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return "", fmt.Errorf("forum input is empty")
	}

	// 1) If user passed an ID, accept if it belongs to this project.
	if containsString(p.ForumChannelIDs, in) {
		return in, nil
	}

	// 2) Otherwise try to match by channel name from stored IDs.
	var matchedID string
	var matchedName string
	var hits int

	for _, id := range p.ForumChannelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		ch, err := s.State.Channel(id)
		if err != nil || ch == nil {
			// Fallback to API
			ch, _ = s.Channel(id)
		}
		if ch == nil {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(ch.Name), in) {
			hits++
			matchedID = id
			matchedName = ch.Name
		}
	}

	switch hits {
	case 0:
		return "", fmt.Errorf("forum not found in project (by id or name): %s", in)
	case 1:
		_ = matchedName
		return matchedID, nil
	default:
		return "", fmt.Errorf("forum name is ambiguous (%d matches). Please use forum channel id instead", hits)
	}
}
