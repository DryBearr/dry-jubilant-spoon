// TODO: tests
// Package kanban provides Kanban-style project organization for Discord.
package kanban

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

const commandKanban = "kanban"

var commandsRegistered atomic.Bool

// UseKanban enables the /kanban commands.
func UseKanban(s *discordgo.Session, logger *slog.Logger, guildID string) error {
	if s == nil {
		return fmt.Errorf("discord session is nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	guildID = strings.TrimSpace(guildID)

	if err := ensureDataDir(); err != nil {
		return err
	}

	s.AddHandlerOnce(func(sess *discordgo.Session, _ *discordgo.Ready) {
		if commandsRegistered.Load() {
			return
		}
		if err := registerKanbanCommands(sess, logger, guildID); err != nil {
			logger.Error("register /kanban failed", "err", err, "guild", guildID)
			return
		}
		commandsRegistered.Store(true)
		logger.Info("kanban commands registered", "guild", guildID)
	})

	s.AddHandler(func(sess *discordgo.Session, i *discordgo.InteractionCreate) {
		handleInteraction(sess, logger, i)
	})

	logger.Info("kanban enabled")
	return nil
}
