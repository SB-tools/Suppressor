package main

import (
	"context"
	"fmt"
	"github.com/disgoorg/json"
	"os"
	"os/signal"
	"strconv"
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
	"github.com/dlclark/regexp2"
	"golang.org/x/exp/slices"
)

var (
	token = os.Getenv("SUPPRESSOR_TOKEN")

	wordlist = []string{
		`(?<!(?:why|how)\s+)is\s+(?:sb|sponsorblock|(?:(?:the\s+)?server))\s+(?:\w+\s+)?(?:down|dead|working)(?:(?:\s+)?\?)?`, // matches "... is (sb|sponsorblock|((the)? server)) (\w+ )?(down|dead|working)(( )?\?)" unless it starts with "why|how"
		`code(?::)?\s+(?:\b5[02]\d\b|\b404\b|undefined)`,                                                                      // "... code(:)? (5xx|404|undefined)"
		`(?:got|get(?:ting)?|is)(?:\s+a)?\s+(?:\b5[02]\d\b|\b404\b|undefined)\s+(?:error|exception|code)`,                     // "... (got|get(ting)?|is)( a)? (5xx|404|undefined) (error|exception|code)"
	}
	regexes []*regexp2.Regexp

	currentTemplate  = "The server is currently treated as **%s**."
	updateTemplate   = "The server is now treated as **%s**."
	incidentTemplate = " Incident resolved after **%.1f** hours."
	sameTemplate     = "The status is already set to **%s**."

	down         = false
	downtimeTime time.Time

	vipRoleID  = snowflake.ID(755511470305050715)
	ajayID     = snowflake.ID(197867122825756672)
	channelIDs = []snowflake.ID{
		snowflake.ID(603643299961503761), // #general
		snowflake.ID(603643180663177220), // #questions
		snowflake.ID(603643256714297374), // #concerns
	}
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
		regexes = append(regexes, regexp2.MustCompile(variant, regexp2.IgnoreCase))
	}

	data, err := os.ReadFile("/home/cane/suppressor/time.txt")
	if err != nil {
		log.Panicf("error while reading file: %s", err)
	}
	i, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if i != 0 {
		down = true
		downtimeTime = time.Unix(int64(i), 0)
	}

	log.Info("suppressor started")

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-s
}

func onReaction(event *events.GuildMessageReactionAdd) {
	emoji := event.Emoji.Name
	if (emoji == "\u2705" || emoji == "\u274C") && isVip(event.Member) {
		client := event.Client().Rest()
		_, _ = client.UpdateMessage(event.ChannelID, event.MessageID, discord.MessageUpdate{
			Flags: json.Ptr(discord.MessageFlagSuppressEmbeds),
		})
	}
}

func onMessage(event *events.GuildMessageCreate) {
	message := event.Message
	if message.WebhookID != nil || message.Author.Bot || isVip(*message.Member) {
		return
	}
	client := event.Client().Rest()
	channelID := event.ChannelID
	if len(message.StickerItems) != 0 {
		_ = client.DeleteMessage(channelID, message.ID)
		return
	}
	if !down || !slices.Contains(channelIDs, channelID) {
		return
	}
	for _, regex := range regexes {
		match, _ := regex.MatchString(message.Content)
		if match {
			_, _ = client.CreateMessage(channelID, discord.NewMessageCreateBuilder().
				SetContentf("SponsorBlock has been down since %s. Stay updated at <https://sponsorblock.works>.",
					discord.TimestampStyleShortDateTime.FormatTime(downtimeTime)).
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
		var unix int64
		if down {
			now := time.Now()
			downtimeTime = now
			unix = now.Unix()
		} else {
			message += fmt.Sprintf(incidentTemplate, time.Now().Sub(downtimeTime).Hours())
		}

		// write timestamp to file
		f, err := os.Create("/home/cane/suppressor/time.txt")
		if err != nil {
			log.Errorf("error while creating file: %s", err)
		} else {
			_, _ = f.WriteString(strconv.FormatInt(unix, 10))
		}
		defer f.Close()

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
