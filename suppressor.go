package main

import (
	"context"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/log"
	"github.com/disgoorg/snowflake"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

var (
	token    = os.Getenv("suppressor")
	wordlist = []string{"502", "bad gateway", "(?:is\\s+)?(?:(?:\\s+)?the\\s+)?(?:sponsor(?:\\s+)?block|sb|server(?:s)?|api)\\s+(?:down|dead|die(?:d)?)", "overloaded", "(?:sponsor(?:\\s+)?block|sb|server(?:s)?|api) crash(?:ed)?",
		"(?:(?:issue|problem)(?:s)?\\s+)(?:with\\s+)?(?:the\\s+)?(?:sponsor(?:\\s+)?block|sb|server(?:s)?|api)", "exclamation mark", "segments\\s+are\\s+(?:not\\s+)?(?:showing|loading)",
		"(?:can't|cannot) submit"}
	down    = false
	regexes []*regexp.Regexp

	commands = []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			CommandName:       "down",
			Description:       "sets whether the server is down (enables wordlist checking)",
			DefaultPermission: true,
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "down",
					Description: "whether the server is down",
					Required:    false,
				},
			},
		},
	}
	guildId   = snowflake.Snowflake("603643120093233162")
	vipRoleId = snowflake.Snowflake("755511470305050715")
)

func main() {
	log.SetLevel(log.LevelInfo)
	log.Info("starting the bot...")
	log.Info("disgo version: ", disgo.Version)

	client, err := disgo.New(token,
		bot.WithGatewayConfigOpts(gateway.WithGatewayIntents(discord.GatewayIntentGuildMessages, discord.GatewayIntentGuildMessageReactions,
			discord.GatewayIntentGuilds)),
		bot.WithCacheConfigOpts(
			cache.WithCacheFlags(cache.FlagGuildTextChannels),
			cache.WithMemberCachePolicy(cache.MemberCachePolicyNone),
		),
		bot.WithEventListeners(&events.ListenerAdapter{
			OnGuildMessageReactionAdd:       onReaction,
			OnGuildMessageCreate:            onMessage,
			OnApplicationCommandInteraction: onSlashCommand,
		}))

	if err != nil {
		log.Fatal("error while building disgo: ", err)
		return
	}

	defer client.Close(context.TODO())

	err = client.ConnectGateway(context.TODO())
	if err != nil {
		log.Fatalf("error while connecting to the gateway: %s", err)
		return
	}

	_, err = client.Rest().Applications().SetGuildCommands(client.ApplicationID(), guildId, commands)
	if err != nil {
		log.Fatalf("error while registering commands: %s", err)
	}

	for _, variant := range wordlist {
		regexes = append(regexes, regexp.MustCompile(variant))
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-s
}

func onReaction(event *events.GuildMessageReactionAddEvent) {
	emoji := event.Emoji.Name
	if (emoji == "\u2705" || emoji == "\u274C") && isVip(&event.Member) {
		suppressed := discord.MessageFlagSuppressEmbeds
		channelService := event.Client().Rest().Channels()
		_, _ = channelService.UpdateMessage(event.ChannelID, event.MessageID, discord.MessageUpdate{
			Flags: &suppressed,
		})
	}
}

func onMessage(event *events.GuildMessageCreateEvent) {
	message := event.Message
	isVip := isVip(message.Member)
	channelsRest := event.Client().Rest().Channels()
	if len(message.Stickers) != 0 && !isVip {
		_ = channelsRest.DeleteMessage(message.ChannelID, message.ID)
		return
	}
	if !down || message.Author.Bot || isVip {
		return
	}
	content := strings.ToLower(message.Content)
	for _, regex := range regexes {
		if regex.MatchString(content) {
			_, _ = channelsRest.CreateMessage(event.ChannelID, discord.NewMessageCreateBuilder().
				SetContent("SponsorBlock is down at the moment. Stay updated at <https://sponsorblock.works>").
				Build())
			return
		}
	}
}

func onSlashCommand(event *events.ApplicationCommandInteractionEvent) {
	data := event.SlashCommandInteractionData()
	if data.CommandName() == "down" {
		messageBuilder := discord.NewMessageCreateBuilder()
		downOption, ok := data.BoolOption("down")
		if !ok {
			_ = event.CreateMessage(messageBuilder.
				SetContentf("The server is currently treated as **%s**.", formatStatus(down)).
				Build())
			return
		}
		if !isVip(&event.Member().Member) {
			_ = event.CreateMessage(messageBuilder.
				SetContent("This command is VIP only.").
				SetEphemeral(true).
				Build())
			return
		}
		down = downOption.Value
		_ = event.CreateMessage(messageBuilder.
			SetContentf("The server is now treated as **%s**.", formatStatus(down)).
			Build())
	}
}

func isVip(member *discord.Member) bool {
	roles := member.RoleIDs
	for _, roleId := range roles {
		if roleId == vipRoleId {
			return true
		}
	}
	return false
}

func formatStatus(downStatus bool) string {
	var status string
	if downStatus {
		status = "offline"
	} else {
		status = "online"
	}
	return status
}
