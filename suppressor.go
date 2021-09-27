package main

import (
	"github.com/DisgoOrg/disgo/core"
	"github.com/DisgoOrg/disgo/discord"
	"github.com/DisgoOrg/disgo/gateway"
	"github.com/DisgoOrg/log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

var (
	token    = os.Getenv("suppressor")
	wordlist = []string{"502", "bad gateway", "(?:is\\s+)?(?:(?:\\s+)?the\\s+)?(?:sponsor(\\s+)?block|sb|server)\\s+(?:down|dead|die(?:d)?)", "overloaded", "(?:sponsorblock|sb|server) crash(?:ed)?",
		"(?:issue(?:s)?\\s+)(?:with\\s+)?(?:the\\s+)?(?:sponsorblock|sb|server)", "exclamation mark", "segments\\s+are\\s+(?:not\\s+)?(?:showing|loading)",
		"(?:can't|cannot) submit"}
	down    = false
	regexes []*regexp.Regexp

	commands = []discord.ApplicationCommandCreate{
		{
			Type:              discord.ApplicationCommandTypeSlash,
			Name:              "down",
			Description:       "sets whether the server is down (enables wordlist checking)",
			DefaultPermission: true,
			Options: []discord.ApplicationCommandOption{
				{
					Type:        discord.ApplicationCommandOptionTypeBoolean,
					Name:        "down",
					Description: "whether the server is down",
					Required:    true,
				},
			},
		},
	}
	guildId              = discord.Snowflake("603643120093233162")
	vipRoleId            = discord.Snowflake("755511470305050715")
	submissionsChannelId = discord.Snowflake("655247785561554945")
)

func main() {
	log.SetLevel(log.LevelInfo)

	disgo, err := core.NewBotBuilder(token).
		SetGatewayConfigOpts(gateway.WithGatewayIntents(discord.GatewayIntentGuildMessages, discord.GatewayIntentGuildMessageReactions,
			discord.GatewayIntentGuilds)).
		SetCacheConfigOpts(
			core.WithCacheFlags(core.CacheFlagTextChannels),
			core.WithMemberCachePolicy(core.MemberCachePolicyNone),
		).
		AddEventListeners(&core.ListenerAdapter{
			OnGuildMessageReactionAdd: onReaction,
			OnGuildMessageCreate:      onMessage,
			OnSlashCommand:            onSlashCommand,
		}).
		Build()

	if err != nil {
		log.Fatal("error while building disgo: ", err)
		return
	}

	defer disgo.Close()

	_, err = disgo.SetGuildCommands(guildId, commands)
	if err != nil {
		log.Fatalf("error while registering commands: %s", err)
	}

	for _, variant := range wordlist {
		regexes = append(regexes, regexp.MustCompile(variant))
	}

	err = disgo.ConnectGateway()
	if err != nil {
		log.Fatal("error while starting disgo: ", err)
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-s
}

func onReaction(event *core.GuildMessageReactionAddEvent) {
	channelId := event.ChannelID
	if event.Emoji.Name == "\u2705" && channelId == submissionsChannelId && isVip(event.Member) {
		suppressed := discord.MessageFlagSuppressEmbeds
		_, _ = event.Message.Update(discord.MessageUpdate{
			Flags: &suppressed,
		})
	}
}

func onMessage(event *core.GuildMessageCreateEvent) {
	message := event.Message
	isVip := isVip(message.Member)
	if len(message.Stickers) != 0 && !isVip {
		_ = message.Delete()
		return
	}
	if !down || message.Author.IsBot || isVip {
		return
	}
	content := strings.ToLower(message.Content)
	for _, regex := range regexes {
		if regex.MatchString(content) {
			_, _ = event.Channel().CreateMessage(core.NewMessageCreateBuilder().
				SetContent("SponsorBlock is down at the moment. Stay updated at <https://status.sponsor.ajay.app/>").
				Build())
			return
		}
	}
}

func onSlashCommand(event *core.SlashCommandEvent) {
	if event.CommandName == "down" {
		messageBuilder := core.NewMessageCreateBuilder()
		if !isVip(event.Member) {
			_ = event.Create(messageBuilder.
				SetContent("This command is VIP only.").
				Build())
			return
		}

		down = event.Options.Get("down").Bool()
		_ = event.Create(messageBuilder.
			SetContentf("server down status has been set to `%t`", down).
			Build())
	}
}

func isVip(member *core.Member) bool {
	roles := member.RoleIDs
	for _, roleId := range roles {
		if roleId == vipRoleId {
			return true
		}
	}
	return false
}
