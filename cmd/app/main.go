package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"dry-jubilant-spoon/bot"
	"dry-jubilant-spoon/bot-features/kanban"

	"github.com/bwmarrin/discordgo"
)

func main() {
	token, guildID, verbose := parseFlags()

	logger := newLogger(verbose)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := bot.Config{
		Token:   token,
		Intents: discordgo.IntentsGuilds,
		GuildID: guildID,
	}

	if err := bot.Start(
		ctx,
		cfg,
		logger,
		func(s *discordgo.Session, l *slog.Logger) error {
			return kanban.UseKanban(s, l, guildID)
		},
	); err != nil {
		logger.Error("bot start failed", "err", err)
		os.Exit(1)
	}

	logger.Info("bot started", "hint", "Ctrl+C to stop")
	<-ctx.Done()
	logger.Info("shutdown complete")
}

func parseFlags() (token, guildID string, verbose bool) {
	flag.StringVar(&token, "token", "", "Discord bot token (or DISCORD_TOKEN env)")
	flag.StringVar(&guildID, "guild", "", "Guild ID for instant command registration (or DISCORD_GUILD_ID env)")
	flag.BoolVar(&verbose, "verbose", false, "Enable debug logging")
	flag.Parse()

	if token == "" {
		token = os.Getenv("DISCORD_TOKEN")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		os.Stderr.WriteString("Discord token is required (use -token or DISCORD_TOKEN)\n")
		os.Exit(2)
	}

	if guildID == "" {
		guildID = os.Getenv("DISCORD_GUILD_ID")
	}
	guildID = strings.TrimSpace(guildID)

	return token, guildID, verbose
}

func newLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
