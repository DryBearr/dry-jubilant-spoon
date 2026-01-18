package kanban

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Hardcoded role permissions (guild-wide).
// Project isolation (hide category / read-only) MUST be enforced via category/forum overwrites.
const (
	KanbanMemberPerms int64 = int64(
		discordgo.PermissionViewChannel |
			discordgo.PermissionReadMessageHistory,
	)

	KanbanLeaderPerms int64 = int64(
		discordgo.PermissionViewChannel |
			discordgo.PermissionReadMessageHistory |
			discordgo.PermissionCreatePublicThreads |
			discordgo.PermissionSendMessagesInThreads |
			discordgo.PermissionManageThreads |
			discordgo.PermissionManageMessages,
	)
)

// ensureProjectRoles creates or reuses two roles for a project:
//   - "<slug>-member"
//   - "<slug>-leader"
func ensureProjectRoles(s *discordgo.Session, guildID, projectSlug string) (memberRoleID, leaderRoleID string, err error) {
	if s == nil {
		return "", "", fmt.Errorf("discord session is nil")
	}
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return "", "", fmt.Errorf("guildID required")
	}

	slug := strings.TrimSpace(projectSlug)
	if slug == "" {
		return "", "", fmt.Errorf("projectSlug required")
	}

	memberName := slug + "-member"
	leaderName := slug + "-leader"

	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return "", "", fmt.Errorf("GuildRoles: %w", err)
	}

	// Reuse if exists.
	for _, r := range roles {
		if r == nil {
			continue
		}
		switch r.Name {
		case memberName:
			memberRoleID = r.ID
		case leaderName:
			leaderRoleID = r.ID
		}
	}

	// Create missing member role (random light readable).
	if memberRoleID == "" {
		color := RandomReadableMemberColor()
		memberRoleID, err = createRole(s, guildID, memberName, KanbanMemberPerms, color, false, false)
		if err != nil {
			return "", "", err
		}
	}

	// Create missing leader role (random deep readable).
	if leaderRoleID == "" {
		color := RandomReadableLeaderColor()
		leaderRoleID, err = createRole(s, guildID, leaderName, KanbanLeaderPerms, color, true, false)
		if err != nil {
			return "", "", err
		}
	}

	return memberRoleID, leaderRoleID, nil
}

// createRole creates and configures a guild role in one call (for your discordgo version).
func createRole(
	s *discordgo.Session,
	guildID string,
	name string,
	perms int64,
	color int, // 0xRRGGBB
	hoist bool,
	mentionable bool,
) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("role name required")
	}

	params := &discordgo.RoleParams{
		Name:        name,
		Permissions: int64Ptr(perms),
		Color:       intPtr(color),
		Hoist:       boolPtr(hoist),
		Mentionable: boolPtr(mentionable),
	}

	r, err := s.GuildRoleCreate(guildID, params)
	if err != nil {
		return "", fmt.Errorf("GuildRoleCreate: %w", err)
	}
	return r.ID, nil
}

func boolPtr(v bool) *bool    { return &v }
func int64Ptr(v int64) *int64 { return &v }
func intPtr(v int) *int       { return &v }
