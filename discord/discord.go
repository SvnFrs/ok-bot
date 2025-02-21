package discord

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/svnfrs/ok-bot/env"
	"github.com/svnfrs/ok-bot/openai"
	"github.com/svnfrs/ok-bot/youtube"
)

var (
	Token = env.GetEnv("DISCORD_BOT_KEY")
)

var mu sync.Mutex

func ptr(s string) *string {
	return &s
}

func StartBot() {
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		log.Fatalf("error creating Discord session: %v", err)
	}

	dg.AddHandler(ready)
	dg.AddHandler(interactionCreate)

	err = dg.Open()

	// deleteAllCommands(dg, "")

	// dg.Close()

	if err != nil {
		log.Fatalf("error opening connection: %v", err)
	}

	log.Println("Bot is now running. Press CTRL+C to exit.")
	select {}
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready!")

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "ping",
			Description: "Replies with Pong!",
		},
		{
			Name:        "ask",
			Description: "Ask a question",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "question",
					Description: "The question you want to ask",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
			},
		},
		{
			Name:        "youtube",
			Description: "Play a Youtube song",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "url",
					Description: "The url of the video you want to hear",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
			},
		},
		{
			Name:        "disconnect",
			Description: "Disconnects the bot from the voice channel",
		},
	}

	for _, command := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", command)
		if err != nil {
			log.Fatalf("Cannot create '%v' command: %v", command.Name, err)
		}
	}
}
func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	switch i.ApplicationCommandData().Name {
	case "ping":
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Pong!",
			},
		})
		if err != nil {
			log.Printf("error responding to interaction: %v", err)
		}
	case "ask":
		// acknowledge the interaction immediately
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		})
		if err != nil {
			log.Printf("error deferring response: %v", err)
			return
		}

		// process the request asynchronously
		go func() {
			question := i.ApplicationCommandData().Options[0].StringValue()
			response := openai.AskChatGPT(question)

			// edit the original response with the actual content
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &response,
			})
			if err != nil {
				log.Printf("error editing response: %v", err)
			}
		}()
	case "youtube":
		// Find the guild (server) the interaction was triggered in
		guild, err := s.State.Guild(i.GuildID)
		if err != nil {
			log.Printf("Error getting guild: %v", err)
			return
		}

		// Find the voice state of the user who triggered the command
		var userVoiceState *discordgo.VoiceState
		for _, vs := range guild.VoiceStates {
			if vs.UserID == i.Member.User.ID {
				userVoiceState = vs
				break
			}
		}

		// Check if the user is in a voice channel
		if userVoiceState == nil {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You need to be in a voice channel to use this command!",
				},
			})
			if err != nil {
				log.Printf("Error responding to interaction: %v", err)
			}
			return
		}

		// Acknowledge the interaction immediately
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		})
		if err != nil {
			log.Printf("Error responding to interaction: %v", err)
			return
		}

		// Get the YouTube URL from the command options
		url := i.ApplicationCommandData().Options[0].StringValue()

		// Download or get existing audio file
		filename, err := youtube.DownloadAudio(url)
		if err != nil {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: ptr(fmt.Sprintf("Error processing YouTube video: %v", err)),
			})
			return
		}

		// Join the voice channel
		voiceConnection, err := s.ChannelVoiceJoin(
			i.GuildID,                // Guild ID
			userVoiceState.ChannelID, // Channel ID
			false,                    // Self mute
			false,                    // Self deaf
		)
		if err != nil {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: ptr(fmt.Sprintf("Error joining voice channel: %v", err)),
			})
			return
		}

		// Update message to show we're playing
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: ptr("Now playing audio..."),
		})

		// Create a done channel to signal when we're done playing
		done := make(chan bool)

		// Play the audio file
		go func() {
			defer func() {
				if voiceConnection != nil {
					voiceConnection.Disconnect()
				}
			}()

			// Add timeout context
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			done := make(chan bool)
			go func() {
				dgvoice.PlayAudioFile(voiceConnection, filename, done)
				close(done)
			}()

			select {
			case <-done:
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: ptr("Finished playing the audio!"),
				})
			case <-ctx.Done():
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: ptr("Playback timed out!"),
				})
			}
		}()

		// Wait for the audio to finish playing
		<-done

		// Update the message once we're done
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: ptr("Finished playing the audio!"),
		})

	case "disconnect":
		// Find the voice connection for this guild
		voiceConnection, ok := s.VoiceConnections[i.GuildID]
		if !ok {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "I'm not in a voice channel!",
				},
			})
			if err != nil {
				log.Printf("Error responding to interaction: %v", err)
			}
			return
		}

		// Disconnect from the voice channel
		err := voiceConnection.Disconnect()
		if err != nil {
			log.Printf("Error disconnecting from voice channel: %v", err)
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Failed to disconnect from voice channel!",
				},
			})
			if err != nil {
				log.Printf("Error responding to interaction: %v", err)
			}
			return
		}

		// Respond to confirm disconnection
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Successfully disconnected from voice channel!",
			},
		})
		if err != nil {
			log.Printf("Error responding to interaction: %v", err)
		}
	}
}

func deleteAllCommands(s *discordgo.Session, guildID string) {
	// Retrieve all commands for the application
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		log.Fatalf("Cannot fetch commands: %v", err)
	}

	// Iterate over the commands and delete each one
	for _, cmd := range commands {
		err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
		if err != nil {
			log.Printf("Cannot delete '%v' command: %v", cmd.Name, err)
		} else {
			log.Printf("Deleted command: %v", cmd.Name)
		}
	}
}
