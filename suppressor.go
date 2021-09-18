package main

import (
	"github.com/DisgoOrg/disgo/core"
	"github.com/DisgoOrg/disgo/discord"
	"github.com/DisgoOrg/disgo/gateway"
	"github.com/DisgoOrg/disgo/rest"
	"github.com/DisgoOrg/log"
	"os"
	"os/signal"
	"syscall"
)

var (
	token                = os.Getenv("suppressor")
	vipRoleId            = discord.Snowflake("755511470305050715")
	submissionsChannelId = discord.Snowflake("655247785561554945")
)

func main() {
	log.SetLevel(log.LevelInfo)

	disgo, err := core.NewBotBuilder(token).
		SetGatewayConfig(gateway.Config{
			GatewayIntents: discord.GatewayIntentGuildMessageReactions | discord.GatewayIntentGuildMessages,
		}).
		AddEventListeners(&core.ListenerAdapter{
			OnGuildMessageReactionAdd: onReaction,
			OnGuildMessageCreate:      onMessage,
		}).SetCacheConfig(core.CacheConfig{
		MemberCachePolicy: core.MemberCachePolicyNone,
		CacheFlags:        core.CacheFlagsNone,
	}).
		Build()

	if err != nil {
		log.Fatal("error while building disgo: ", err)
		return
	}

	defer disgo.Close()

	err = disgo.Connect()
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
		channelService := event.Bot().RestServices.ChannelService()
		suppressed := discord.MessageFlagSuppressEmbeds
		_, _ = channelService.UpdateMessage(channelId, event.MessageID, discord.MessageUpdate{
			Flags: &suppressed,
		})
	}
}

func onMessage(event *core.GuildMessageCreateEvent) {
	message := event.Message
	if len(message.Stickers) != 0 && !isVip(message.Member) {
		_ = message.Delete(rest.WithReason("Stickers are not allowed"))
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
