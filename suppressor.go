package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/log"
	"github.com/disgoorg/snowflake/v2"
	"golang.org/x/exp/slices"
)

var (
	token    = os.Getenv("SUPPRESSOR_TOKEN")
	wordlist = []string{`\b5(?:\d{2})\b`, `bad gateway`, `(?:is\s+)?(?:(?:\s+)?the\s+)?(?:sponsor(?:\s+)?block|sb|server(?:s)?|api)\s+(?:down|dead|die(?:d)?)`, `overloaded`, `(?:sponsor(?:\s+)?block|sb|server(?:s)?|api) crash(?:ed)?`,
		`(?:(?:issue|problem)(?:s)?\s+)(?:with\s+)?(?:the\s+)?(?:sponsor(?:\s+)?block|sb|server(?:s)?|api)`, `exclamation mark`, `segments\s+are\s+(?:not\s+)?(?:showing|loading)`,
		`(?:can't|cannot) submit`, `\b404\b`}
	currentTemplate  = "The server is currently treated as **%s**."
	updateTemplate   = "The server is now treated as **%s**."
	incidentTemplate = " Incident resolved after **%.1f** hours."
	sameTemplate     = "The status is already set to **%s**."
	down             = false
	regexes          []*regexp.Regexp
	downtimeTime     time.Time
	vipRoleID        = snowflake.ID(755511470305050715)
	ajayID           = snowflake.ID(197867122825756672)
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
		client := event.Client().Rest()
		_, _ = client.UpdateMessage(event.ChannelID, event.MessageID, discord.MessageUpdate{
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
	client := event.Client().Rest()
	channelID := event.ChannelID
	if len(message.Stickers) != 0 && !isVip(member) {
		_ = client.DeleteMessage(channelID, message.ID)
		return
	}
	if !down || isVip(member) {
		return
	}
	content := strings.ToLower(message.Content)
	for _, regex := range regexes {
		if regex.MatchString(content) {
			_, _ = client.CreateMessage(channelID, discord.NewMessageCreateBuilder().
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
		formatted := formatStatus(down)
		if !ok {
			_ = event.CreateMessage(messageBuilder.
				SetContentf(currentTemplate, formatted).
				Build())
			return
		}
		member := event.Member()
		if !isVip(member.Member) {
			_ = event.CreateMessage(messageBuilder.
				SetContent("This command is VIP only.").
				SetEphemeral(true).
				Build())
			return
		}
		if down == downOption {
			_ = event.CreateMessage(messageBuilder.
				SetContentf(sameTemplate, formatted).
				SetEphemeral(true).
				Build())
			return
		}
		down = downOption
		message := fmt.Sprintf(updateTemplate, formatStatus(down))
		if member.User.ID == ajayID {
			if down {
				message += " Have fun, Ajay!"
			} else {
				message += " Hope you had fun, Ajay."
			}
		}
		if down {
			downtimeTime = time.Now()
		} else {
			message += fmt.Sprintf(incidentTemplate, time.Now().Sub(downtimeTime).Hours())
		}
		_ = event.CreateMessage(messageBuilder.
			SetContentf(message).
			Build())
	}
}

func isVip(member discord.Member) bool {
	return slices.Contains(member.RoleIDs, vipRoleID)
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
