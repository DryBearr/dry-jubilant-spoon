package kanban

import (
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

func kanbanCommandDef() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        commandKanban,
		Description: "Kanban projects",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "create",
				Description: "Create a project category",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "project",
						Description: "Project name",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "delete",
				Description: "Delete a project (category, forums, roles, and saved data)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "project",
						Description: "Project slug or name",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add-member",
				Description: "Add a user to a project (grants member role)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "project",
						Description: "Project slug or name",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to add",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove-member",
				Description: "Remove a user from a project (revokes member/leader roles)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "project",
						Description: "Project slug or name",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to remove",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "create-forum",
				Description: "Create a new forum channel under a project category",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "project",
						Description: "Project slug or name",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Forum name (e.g. design, backend, bugs)",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "delete-forum",
				Description: "Delete a forum channel from a project",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "project",
						Description: "Project slug or name",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "forum",
						Description: "Forum channel ID or forum name",
						Required:    true,
					},
				},
			},

			// ---------------------------
			// Task workflow (thread-based)
			// ---------------------------

			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "task-take",
				Description: "Take task (only when tag is ToDo)",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "task-done",
				Description: "Move to WaitingForApprove (only when tag is InProgress)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "description",
						Description: "What to review / what changed",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "task-approve",
				Description: "Approve task (only when tag is WaitingForApprove)",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "task-revoke",
				Description: "Back to InProgress (only when tag is WaitingForApprove)",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "task-surrender",
				Description: "Surrender task (InProgress -> ToDo)",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "task-init",
				Description: "Init task in current thread (create status panel)",
			},
		},
	}
}

func registerKanbanCommands(s *discordgo.Session, logger *slog.Logger, guildID string) error {
	if s.State == nil || s.State.User == nil || s.State.User.ID == "" {
		return fmt.Errorf("bot user not ready (State.User.ID empty)")
	}

	appID := s.State.User.ID
	cmd := kanbanCommandDef()

	if guildID != "" {
		_, err := s.ApplicationCommandCreate(appID, guildID, cmd)
		return err
	}

	if len(s.State.Guilds) == 0 {
		_, err := s.ApplicationCommandCreate(appID, "", cmd) // global (slow)
		return err
	}

	for _, g := range s.State.Guilds {
		if g == nil || g.ID == "" {
			continue
		}
		if _, err := s.ApplicationCommandCreate(appID, g.ID, cmd); err != nil && logger != nil {
			logger.Error("create guild command failed", "guild", g.ID, "err", err)
		}
	}
	return nil
}
