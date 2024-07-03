package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
	"github.com/lmittmann/tint"
	"golang.org/x/exp/slices"
)

const (
	xEmoji       = "❌"
	successEmoji = "✅"

	vipRoleID     = snowflake.ID(755511470305050715)
	dearrowUserID = snowflake.ID(1114610194438181035)
)

var dearrowReplies = make(map[snowflake.ID]snowflake.ID)

func main() {
	logger := tint.NewHandler(os.Stdout, &tint.Options{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(logger))

	client, err := disgo.New(os.Getenv("SUPPRESSOR_TOKEN"),
		bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentGuildMessages, gateway.IntentGuildMessageReactions)),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagsNone)),
		bot.WithEventListeners(&events.ListenerAdapter{
			OnGuildMessageReactionAdd: onReaction,
			OnGuildMessageCreate:      onMessage,
		}))
	if err != nil {
		panic(err)
	}

	defer client.Close(context.TODO())

	if err := client.OpenGateway(context.TODO()); err != nil {
		panic(err)
	}

	slog.Info("suppressor bot is now running.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-s
}

func onReaction(event *events.GuildMessageReactionAdd) {
	emoji := event.Emoji.Name
	if emoji == nil {
		return
	}
	if (*emoji != xEmoji && *emoji != successEmoji) || !isVip(event.Member) {
		return
	}
	client := event.Client().Rest()
	if replyID, ok := dearrowReplies[event.MessageID]; ok {
		if err := client.DeleteMessage(event.ChannelID, replyID); err != nil {
			slog.Error("error while deleting DeArrow reply",
				slog.Any("reply.id", replyID),
				slog.Any("channel.id", event.ChannelID),
				tint.Err(err))
		}
		delete(dearrowReplies, event.MessageID)
		return
	}
	_, err := client.UpdateMessage(event.ChannelID, event.MessageID, discord.NewMessageUpdateBuilder().
		SetSuppressEmbeds(true).
		Build())
	if err != nil {
		slog.Error("error while suppressing embeds",
			slog.Any("message.id", event.MessageID),
			slog.Any("channel.id", event.ChannelID),
			tint.Err(err))
	}
}

func onMessage(event *events.GuildMessageCreate) {
	message := event.Message
	if message.Member == nil {
		return
	}
	if isVip(*message.Member) {
		return
	}
	if message.Author.ID == dearrowUserID {
		dearrowReplies[*message.MessageReference.MessageID] = message.ID
		return
	}
	if len(message.StickerItems) != 0 {
		if err := event.Client().Rest().DeleteMessage(event.ChannelID, message.ID); err != nil {
			slog.Error("error while deleting message with stickers",
				slog.Any("message.id", message.ID),
				slog.Any("user.id", message.Author.ID),
				tint.Err(err))
		}
	}
}

func isVip(member discord.Member) bool {
	return slices.Contains(member.RoleIDs, vipRoleID)
}
