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
	"github.com/disgoorg/snowflake/v2"
	"golang.org/x/exp/slices"
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
	down      = false
	regexes   []*regexp.Regexp
	vipRoleId = snowflake.ID(755511470305050715)
)

func main() {
	log.SetLevel(log.LevelInfo)
	log.Info("starting the bot...")
	log.Info("disgo version: ", disgo.Version)

	client, err := disgo.New(token,
		bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentGuildMessages, gateway.IntentGuildMessageReactions, gateway.IntentGuilds, gateway.IntentMessageContent)),
		bot.WithCacheConfigOpts(cache.WithCacheFlags(cache.FlagChannels)),
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

	err = client.OpenGateway(context.TODO())
	if err != nil {
		log.Fatalf("error while connecting to the gateway: %s", err)
		return
	}

	for _, variant := range wordlist {
		regexes = append(regexes, regexp.MustCompile(variant))
	}

	log.Info("suppressor started")

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-s
}

func onReaction(event *events.GuildMessageReactionAdd) {
	emoji := event.Emoji.Name
	if (emoji == "\u2705" || emoji == "\u274C") && isVip(event.Member) {
		suppressed := discord.MessageFlagSuppressEmbeds
		channelService := event.Client().Rest()
		_, _ = channelService.UpdateMessage(event.ChannelID, event.MessageID, discord.MessageUpdate{
			Flags: &suppressed,
		})
	}
}

func onMessage(event *events.GuildMessageCreate) {
	message := event.Message
	if message.WebhookID != nil || message.Author.Bot { // vip check should only run when needed
		return
	}
	member := *message.Member
	channelsRest := event.Client().Rest()
	if len(message.Stickers) != 0 && !isVip(member) {
		_ = channelsRest.DeleteMessage(message.ChannelID, message.ID)
		return
	}
	if !down || isVip(member) {
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

func onSlashCommand(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()
	if data.CommandName() == "down" {
		messageBuilder := discord.NewMessageCreateBuilder()
		downOption, ok := data.OptBool("down")
		if !ok {
			_ = event.CreateMessage(messageBuilder.
				SetContentf("The server is currently treated as **%s**.", formatStatus(down)).
				Build())
			return
		}
		if !isVip(event.Member().Member) {
			_ = event.CreateMessage(messageBuilder.
				SetContent("This command is VIP only.").
				SetEphemeral(true).
				Build())
			return
		}
		down = downOption
		_ = event.CreateMessage(messageBuilder.
			SetContentf("The server is now treated as **%s**.", formatStatus(down)).
			Build())
	}
}

func isVip(member discord.Member) bool {
	return slices.Contains(member.RoleIDs, vipRoleId)
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
