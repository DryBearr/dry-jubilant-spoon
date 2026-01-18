# dry-jubilant-spoon

## Launch

### Run with flags

```bash
go run ./cmd/app -token \"YOUR_DISCORD_BOT_TOKEN\" -guild \"YOUR_GUILD_ID\" -verbose
```

Flags:
- -token   : Discord bot token (required if DISCORD_TOKEN env is not set)
- -guild   : Guild ID for instant slash-command registration (optional)
- -verbose : Enable debug logging

### Run without flags (environment variables)

```bash
export DISCORD_TOKEN=\"YOUR_DISCORD_BOT_TOKEN\"
export DISCORD_GUILD_ID=\"YOUR_GUILD_ID\"   # optional
go run ./cmd/app
```

### Mixed usage (token via env, flags for the rest)
```bash

export DISCORD_TOKEN=\"YOUR_DISCORD_BOT_TOKEN\"
go run ./cmd/app -guild \"YOUR_GUILD_ID\" -verbose
```
