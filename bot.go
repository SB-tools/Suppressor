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
	"syscall"
)

const (
	vipRoleID     = snowflake.ID(755511470305050715)
	dearrowUserID = snowflake.ID(1114610194438181035)
)

var dearrowReplies = make(map[snowflake.ID]snowflake.ID)

func main() {
	log.SetLevel(log.LevelInfo)
	log.Info("starting the bot...")
	log.Info("disgo version: ", disgo.Version)

	client, err := disgo.New(os.Getenv("SUPPRESSOR_TOKEN"),
		bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentGuildMessages, gateway.IntentGuildMessageReactions)),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagsNone)),
		bot.WithEventListeners(&events.ListenerAdapter{
			OnGuildMessageReactionAdd: onReaction,
			OnGuildMessageCreate:      onMessage,
		}))

	if err != nil {
		log.Fatal("error while building disgo: ", err)
	}

	defer client.Close(context.TODO())

	if err := client.OpenGateway(context.TODO()); err != nil {
		log.Fatal("error while connecting to the gateway: ", err)
	}

	log.Info("suppressor bot is now running.")

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-s
}

func onReaction(event *events.GuildMessageReactionAdd) {
	emoji := *event.Emoji.Name
	if (emoji == "\u2705" || emoji == "\u274C") && isVip(event.Member) {
		messageID := event.MessageID
		channelID := event.ChannelID
		client := event.Client().Rest()
		if replyID, ok := dearrowReplies[messageID]; ok {
			err := client.DeleteMessage(channelID, replyID)
			if err != nil {
				log.Errorf("there was an error while deleting a DeArrow reply (%d): ", replyID, err)
			}
			delete(dearrowReplies, messageID)
		} else {
			_, err := client.UpdateMessage(channelID, messageID, discord.NewMessageUpdateBuilder().
				SetSuppressEmbeds(true).
				Build())
			if err != nil {
				log.Errorf("there was an error while suppressing embeds for message %d: ", messageID, err)
			}
		}
	}
}

func onMessage(event *events.GuildMessageCreate) {
	message := event.Message
	if message.WebhookID == nil && isVip(*message.Member) {
		return
	}
	client := event.Client().Rest()
	channelID := event.ChannelID
	messageID := message.ID
	if len(message.StickerItems) != 0 {
		err := client.DeleteMessage(channelID, messageID)
		if err != nil {
			log.Errorf("there was an error while deleting message %d: ", messageID, err)
		}
		return
	}
	if message.Author.ID == dearrowUserID {
		dearrowReplies[*message.MessageReference.MessageID] = messageID
	}
}

func isVip(member discord.Member) bool {
	return slices.Contains(member.RoleIDs, vipRoleID)
}
