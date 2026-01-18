// Package bot provides a minimal Discord connection layer.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

type Config struct {
	Token   string
	Intents discordgo.Intent
	GuildID string
}

type SetupFunc func(s *discordgo.Session, logger *slog.Logger) error

var sess atomic.Value // stores *discordgo.Session

func setSession(s *discordgo.Session) { sess.Store(s) }

// Session returns the initialized Discord session.
func Session() (*discordgo.Session, error) {
	v := sess.Load()
	if v == nil {
		return nil, errors.New("session not initialized")
	}
	return v.(*discordgo.Session), nil
}

// Start connects to Discord and runs until ctx is cancelled.
func Start(ctx context.Context, cfg Config, logger *slog.Logger, setup ...SetupFunc) error {
	l := logger
	if l == nil {
		l = slog.Default()
	}

	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return fmt.Errorf("token required")
	}

	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return err
	}

	if cfg.Intents != 0 {
		s.Identify.Intents = cfg.Intents
	} else {
		s.Identify.Intents = discordgo.IntentsGuilds
	}

	wirePing(s, l, strings.TrimSpace(cfg.GuildID))

	for _, fn := range setup {
		if fn == nil {
			continue
		}
		if err := fn(s, l); err != nil {
			return err
		}
	}

	if err := s.Open(); err != nil {
		return err
	}
	setSession(s)

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	return nil
}

func wirePing(s *discordgo.Session, l *slog.Logger, guildID string) {
	s.AddHandler(func(sess *discordgo.Session, r *discordgo.Ready) {
		l.Info("ready", "user", r.User.Username, "discriminator", r.User.Discriminator)

		targetGuild := guildID
		if targetGuild == "" {
			targetGuild = "" // global (slow)
		}

		_, err := sess.ApplicationCommandCreate(sess.State.User.ID, targetGuild, &discordgo.ApplicationCommand{
			Name:        "ping",
			Description: "pong",
		})
		if err != nil {
			l.Error("register /ping failed", "err", err, "guild", targetGuild)
		}
	})

	s.AddHandler(func(sess *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "ping" {
			return
		}

		if err := sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "pong",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		}); err != nil {
			l.Error("respond to /ping failed", "err", err)
		}
	})
}
