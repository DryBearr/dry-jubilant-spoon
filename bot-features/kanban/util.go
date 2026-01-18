package kanban

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func createProjectCategory(
	s *discordgo.Session,
	guildID, projectName string,
	private bool,
	allowViewRoleIDs ...string,
) (string, error) {
	if s == nil {
		return "", fmt.Errorf("discord session is nil")
	}
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return "", fmt.Errorf("guild required")
	}

	name := strings.TrimSpace(projectName)
	if name == "" {
		return "", fmt.Errorf("project name required")
	}

	var overwrites []*discordgo.PermissionOverwrite
	if private {
		var err error
		overwrites, err = buildPrivateCategoryOverwrites(guildID, allowViewRoleIDs)
		if err != nil {
			return "", err
		}
	}

	ch, err := s.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:                 name,
		Type:                 discordgo.ChannelTypeGuildCategory,
		PermissionOverwrites: overwrites,
	})
	if err != nil {
		return "", err
	}
	return ch.ID, nil
}

// buildPrivateCategoryOverwrites hides category from everyone except provided role IDs.
func buildPrivateCategoryOverwrites(guildID string, allowViewRoleIDs []string) ([]*discordgo.PermissionOverwrite, error) {
	// @everyone role id == guildID
	everyoneRoleID := strings.TrimSpace(guildID)
	if everyoneRoleID == "" {
		return nil, fmt.Errorf("guildID required for privacy overwrites")
	}

	// Clean + deduplicate role IDs.
	seen := make(map[string]struct{}, len(allowViewRoleIDs))
	clean := make([]string, 0, len(allowViewRoleIDs))
	for _, id := range allowViewRoleIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}

	if len(clean) == 0 {
		return nil, fmt.Errorf("private category requires at least one allowed role id")
	}

	view := int64(discordgo.PermissionViewChannel)

	overwrites := make([]*discordgo.PermissionOverwrite, 0, 1+len(clean))

	// Deny everyone.
	overwrites = append(overwrites, &discordgo.PermissionOverwrite{
		ID:   everyoneRoleID,
		Type: discordgo.PermissionOverwriteTypeRole,
		Deny: view,
	})

	// Allow only specific roles.
	for _, id := range clean {
		overwrites = append(overwrites, &discordgo.PermissionOverwrite{
			ID:    id,
			Type:  discordgo.PermissionOverwriteTypeRole,
			Allow: view,
		})
	}

	return overwrites, nil
}

// createProjectForum creates a Forum channel under the given category and returns its channel ID.
func createProjectForum(s *discordgo.Session, guildID, categoryID, forumName string) (string, error) {
	if strings.TrimSpace(guildID) == "" {
		return "", fmt.Errorf("guild required")
	}
	if strings.TrimSpace(categoryID) == "" {
		return "", fmt.Errorf("categoryID required")
	}

	name := strings.TrimSpace(forumName)
	if name == "" {
		name = "general"
	}

	ch, err := s.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     name,
		Type:     discordgo.ChannelTypeGuildForum,
		ParentID: categoryID,
	})
	if err != nil {
		return "", err
	}
	return ch.ID, nil
}

func getSubOptionString(sub *discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range sub.Options {
		if o.Name == name {
			return o.StringValue()
		}
	}
	return ""
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// RandomReadableMemberColor returns a bright, readable role color for "member".
func RandomReadableMemberColor() int {
	// Light range for easy readability.
	return randomReadableRoleColor(0.68, 0.82, 0.55, 0x93C5FD) // fallback: light blue
}

// RandomReadableLeaderColor returns a deeper but still readable role color for "leader".
func RandomReadableLeaderColor() int {
	// Deeper range but not too dark.
	return randomReadableRoleColor(0.42, 0.56, 0.35, 0x6D28D9) // fallback: deep violet
}

// randomReadableRoleColor generates a random vivid color with constrained lightness and minimum luminance.
// - lightMin/lightMax: HSL lightness range [0..1]
// - lumMin: relative luminance threshold [0..1]
// - fallback: used if random attempts fail
func randomReadableRoleColor(lightMin, lightMax, lumMin float64, fallback int) int {
	// High saturation keeps colors vivid (not gray).
	const satMin, satMax = 0.75, 0.95

	for range 24 {
		h := randFloat64()                          // [0..1)
		s := satMin + randFloat64()*(satMax-satMin) // [satMin..satMax]
		l := lightMin + randFloat64()*(lightMax-lightMin)

		r, g, b := hslToRGB(h, s, l)
		lum := relativeLuminance(r, g, b)

		if lum >= lumMin {
			return (r << 16) | (g << 8) | b
		}
	}

	return fallback
}

// randFloat64 returns a random float64 in [0,1).
// Uses crypto/rand so it's concurrency-safe without global locks.
func randFloat64() float64 {
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		// extremely rare; fall back to deterministic constant-ish
		return 0.5
	}
	u := binary.LittleEndian.Uint64(buf[:])
	// Convert to [0,1): take top 53 bits (float64 mantissa)
	const max = float64(1 << 53)
	return float64(u>>11) / max
}

// hslToRGB converts HSL in [0..1] to RGB in [0..255] each.
func hslToRGB(h, s, l float64) (int, int, int) {
	h = wrap01(h)
	s = clamp01(s)
	l = clamp01(l)

	if s == 0 {
		// gray
		v := int(math.Round(l * 255))
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q

	r := hueToRGB(p, q, h+1.0/3.0)
	g := hueToRGB(p, q, h)
	b := hueToRGB(p, q, h-1.0/3.0)

	return int(math.Round(r * 255)), int(math.Round(g * 255)), int(math.Round(b * 255))
}

func hueToRGB(p, q, t float64) float64 {
	t = wrap01(t)
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
}

// relativeLuminance computes WCAG relative luminance for sRGB values in [0..255].
func relativeLuminance(r, g, b int) float64 {
	R := srgbToLinear(float64(r) / 255.0)
	G := srgbToLinear(float64(g) / 255.0)
	B := srgbToLinear(float64(b) / 255.0)
	return 0.2126*R + 0.7152*G + 0.0722*B
}

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func wrap01(v float64) float64 {
	v = math.Mod(v, 1)
	if v < 0 {
		v += 1
	}
	return v
}

func isLeaderForProject(i *discordgo.InteractionCreate, authorID string, p Project) bool {
	// Preferred: Discord role check (real-time).
	if i != nil && i.Member != nil && strings.TrimSpace(p.LeaderRoleID) != "" {
		if memberHasRole(i.Member, p.LeaderRoleID) {
			return true
		}
	}
	// Fallback: stored project membership map.
	if authorID != "" && p.Members != nil {
		return p.Members[authorID] == Leader
	}
	return false
}

func memberHasRole(m *discordgo.Member, roleID string) bool {
	if m == nil || roleID == "" {
		return false
	}
	return slices.Contains(m.Roles, roleID)
}

func getSubOptionUserID(sub *discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range sub.Options {
		if o.Name != name {
			continue
		}

		// For USER option, discordgo typically stores ID in o.Value as string.
		if v, ok := o.Value.(string); ok {
			return v
		}

		// Some discordgo versions may expose it differently; keep safe fallback.
		// (If your version supports o.UserValue(s), you can use it, but this works without session.)
		return ""
	}
	return ""
}

// createProjectForumWithKanbanTags creates a Forum channel under the given category,
// and configures default Kanban tags.
//
// discordgo v0.29.0: tags must be set via ChannelEditComplex after creation.
//
// Returns:
// - forum channel ID
// - map[tagName]tagID (IDs are needed when you apply tags on forum posts)
func createProjectForumWithKanbanTags(
	s *discordgo.Session,
	guildID, categoryID, forumName string,
) (forumID string, tagIDs map[string]string, err error) {
	if s == nil {
		return "", nil, fmt.Errorf("discord session is nil")
	}
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return "", nil, fmt.Errorf("guild required")
	}
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return "", nil, fmt.Errorf("categoryID required")
	}

	name := strings.TrimSpace(forumName)
	if name == "" {
		name = "general"
	}

	// 1) Create forum channel (tags are NOT reliably created here in v0.29.0)
	ch, err := s.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     name,
		Type:     discordgo.ChannelTypeGuildForum,
		ParentID: categoryID,
	})
	if err != nil {
		return "", nil, err
	}
	if ch == nil || strings.TrimSpace(ch.ID) == "" {
		return "", nil, fmt.Errorf("discord returned empty channel")
	}

	// 2) Set tags via ChannelEditComplex (v0.29.0-compatible)
	edited, err := s.ChannelEditComplex(ch.ID, &discordgo.ChannelEdit{
		AvailableTags: kanbanDefaultForumTags(),
	})
	if err != nil {
		// forum exists but tag setup failed
		return ch.ID, nil, fmt.Errorf("forum created but tag setup failed: %w", err)
	}

	// 3) Read tags back (from edited result; if nil, fallback to Channel fetch)
	if edited == nil {
		edited, _ = s.Channel(ch.ID)
	}

	tagIDs = make(map[string]string, 8)
	if edited != nil && len(edited.AvailableTags) > 0 {
		for _, t := range edited.AvailableTags {
			switch tt := any(t).(type) {
			case *discordgo.ForumTag:
				if tt != nil && tt.Name != "" && tt.ID != "" {
					tagIDs[tt.Name] = tt.ID
				}
			case discordgo.ForumTag:
				if tt.Name != "" && tt.ID != "" {
					tagIDs[tt.Name] = tt.ID
				}
			}
		}
	}

	return ch.ID, tagIDs, nil
}

// kanbanDefaultForumTags defines your Kanban statuses.
// Emojis are optional but make tags easier to read.
func kanbanDefaultForumTags() *[]discordgo.ForumTag {
	return &[]discordgo.ForumTag{
		{Name: "ToDo", Moderated: false, EmojiName: "🟥"},
		{Name: "InProgress", Moderated: false, EmojiName: "🟨"},
		{Name: "WaitingForApprove", Moderated: false, EmojiName: "🟦"},
		{Name: "Done", Moderated: false, EmojiName: "🟩"},
		{Name: "Blocked", Moderated: false, EmojiName: "⛔"},
		{Name: "Bug", Moderated: false, EmojiName: "🐞"},
		{Name: "Idea", Moderated: false, EmojiName: "💡"},
	}
}

func containsString(xs []string, v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	for _, x := range xs {
		if strings.TrimSpace(x) == v {
			return true
		}
	}
	return false
}

func removeString(xs []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" || len(xs) == 0 {
		return xs
	}
	out := xs[:0]
	for _, x := range xs {
		if strings.TrimSpace(x) == v {
			continue
		}
		out = append(out, x)
	}
	return out
}

func mustGuild(i *discordgo.InteractionCreate) error {
	if strings.TrimSpace(i.GuildID) == "" {
		return fmt.Errorf("guild required")
	}
	return nil
}

func statusToTagName(st TaskStatus) string {
	switch st {
	case TaskToDo:
		return "ToDo"
	case TaskInProgress:
		return "InProgress"
	case TaskWaitingForApprove:
		return "WaitingForApprove"
	case TaskDone:
		return "Done"
	default:
		return ""
	}
}

// ensureForumTagMapping makes sure p.ForumTagIDs[forumID] exists.
// If missing, it fetches the forum channel tags and stores them.
func ensureForumTagMapping(s *discordgo.Session, p Project, forumID string) (Project, error) {
	forumID = strings.TrimSpace(forumID)
	if forumID == "" {
		return p, fmt.Errorf("forumID required")
	}
	if p.ForumTagIDs == nil {
		p.ForumTagIDs = make(map[string]map[string]string)
	}
	if m := p.ForumTagIDs[forumID]; len(m) > 0 {
		return p, nil
	}

	// Fetch forum channel to read AvailableTags.
	ch, err := getChannelSafe(s, forumID)
	if err != nil {
		return p, err
	}
	if ch == nil {
		return p, fmt.Errorf("forum channel not found")
	}

	tagIDs := make(map[string]string, 16)
	for _, t := range ch.AvailableTags {
		switch tt := any(t).(type) {
		case *discordgo.ForumTag:
			if tt != nil && tt.Name != "" && tt.ID != "" {
				tagIDs[tt.Name] = tt.ID
			}
		case discordgo.ForumTag:
			if tt.Name != "" && tt.ID != "" {
				tagIDs[tt.Name] = tt.ID
			}
		}
	}

	if len(tagIDs) == 0 {
		return p, fmt.Errorf("forum has no tags configured")
	}

	p.ForumTagIDs[forumID] = tagIDs
	return p, nil
}

func applyStatusTagToThread(s *discordgo.Session, p Project, forumID, threadID string, status TaskStatus) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("threadID required")
	}
	forumID = strings.TrimSpace(forumID)
	if forumID == "" {
		return fmt.Errorf("forumID required")
	}

	tagName := statusToTagName(status)
	if tagName == "" {
		return fmt.Errorf("unknown status: %s", status)
	}

	m := p.ForumTagIDs[forumID]
	if len(m) == 0 {
		return fmt.Errorf("no tag mapping for forum")
	}

	tagID := strings.TrimSpace(m[tagName])
	if tagID == "" {
		return fmt.Errorf("missing tag id for %q in forum %s", tagName, forumID)
	}

	// Apply exactly one status tag.
	_, err := s.ChannelEditComplex(threadID, &discordgo.ChannelEdit{
		AppliedTags: &[]string{tagID},
	})
	return err
}
