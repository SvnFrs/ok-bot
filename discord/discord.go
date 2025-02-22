package discord

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/svnfrs/ok-bot/env"
	"github.com/svnfrs/ok-bot/openai"
	"github.com/svnfrs/ok-bot/queue"
	"github.com/svnfrs/ok-bot/youtube"
)

var (
	Token = env.GetEnv("DISCORD_BOT_KEY")
)

var (
	musicQueue *queue.MusicQueue
	currentVC  *discordgo.VoiceConnection
	queueMutex sync.Mutex
)

var mu sync.Mutex

func ptr(s string) *string {
	return &s
}

func init() {
	musicQueue = queue.NewMusicQueue()
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
			Name:        "ok",
			Description: "List current commands",
		},
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
			Name:        "queue",
			Description: "Show the current music queue",
		},
		{
			Name:        "skip",
			Description: "Skip the current song",
		},
		{
			Name:        "stop",
			Description: "Stop the current song",
		},
		{
			Name:        "resume",
			Description: "Resume the current song",
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
	case "ok":
		commandList := `Available Commands:
/ok - Show this list of commands
/ping - Check if bot is responsive
/ask [question] - Ask ChatGPT a question
/youtube [url] - Play audio from a YouTube video
/queue - Show the current music queue
/skip - Skip the current song
/stop - Pause the current playback
/resume - Resume paused playback
/disconnect - Disconnect bot from voice channel`

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: commandList,
			},
		})
		if err != nil {
			log.Printf("error responding to interaction: %v", err)
		}
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

		queueMutex.Lock()
		if currentVC == nil {
			// Join the voice channel if not already connected
			currentVC, err = s.ChannelVoiceJoin(
				i.GuildID,
				userVoiceState.ChannelID,
				false,
				false,
			)
			if err != nil {
				queueMutex.Unlock()
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: ptr(fmt.Sprintf("Error joining voice channel: %v", err)),
				})
				return
			}
		}
		queueMutex.Unlock()

		// Add the song to the queue
		musicQueue.Add(queue.Song{
			URL:      url,
			Filename: filename,
		})

		// Start playing if nothing is currently playing
		if !musicQueue.IsPlaying() {
			go playNextSong(s, i.GuildID, i.ChannelID)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: ptr("Now playing audio..."),
			})
		} else {
			// If something is playing, inform that it was added to queue
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: ptr("Added to queue! Use /queue to see the current queue."),
			})
		}

	case "queue":
		songs := musicQueue.List()
		if len(songs) == 0 {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "The queue is empty!",
				},
			})
			return
		}

		content := "Current queue:\n"
		for idx, song := range songs {
			content += fmt.Sprintf("%d. %s\n", idx+1, song.URL)
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
			},
		})

	case "skip":
		if !musicQueue.IsPlaying() {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Nothing is playing!",
				},
			})
			return
		}

		musicQueue.Stop()
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Skipped!",
			},
		})

	case "stop":
		if !musicQueue.IsPlaying() {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Nothing is playing!",
				},
			})
			return
		}

		musicQueue.Stop()
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Playback paused!",
			},
		})

	// Update the resume case in interactionCreate
	case "resume":
		if !musicQueue.IsPaused() {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Playback is not paused!",
				},
			})
			return
		}

		musicQueue.Resume()
		go playNextSong(s, i.GuildID, i.ChannelID)

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Playback resumed!",
			},
		})

	// Update the disconnect case in interactionCreate
	case "disconnect":
		queueMutex.Lock()
		if currentVC != nil {
			musicQueue.Stop()
			musicQueue.Clear()
			err := currentVC.Disconnect()
			currentVC = nil
			queueMutex.Unlock()

			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Error disconnecting from voice channel!",
					},
				})
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Disconnected from voice channel!",
				},
			})
			return
		}
		queueMutex.Unlock()

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Not connected to a voice channel!",
			},
		})
	}
}

func playNextSong(s *discordgo.Session, guildID string, channelID string) {
	for {
		if musicQueue.IsPaused() {
			time.Sleep(time.Second)
			continue
		}

		if !musicQueue.IsPlaying() {
			var song queue.Song
			var ok bool

			// If there's no current song, get the next one from queue
			if currentSong := musicQueue.GetCurrentSong(); currentSong == nil {
				song, ok = musicQueue.Next()
				if !ok {
					return
				}
				musicQueue.SetCurrentSong(&song)
			} else {
				// Resume playing the current song
				song = *currentSong
				ok = true
			}

			musicQueue.SetPlaying(true)
			done := make(chan bool)

			// Ensure voice connection is still valid
			if currentVC == nil || currentVC.Ready == false {
				var err error
				// Get the voice channel ID from the guild
				guild, _ := s.State.Guild(guildID)
				var channelID string
				for _, vs := range guild.VoiceStates {
					if vs.UserID == s.State.User.ID {
						channelID = vs.ChannelID
						break
					}
				}

				if channelID != "" {
					currentVC, err = s.ChannelVoiceJoin(guildID, channelID, false, false)
					if err != nil {
						log.Printf("Error rejoining voice channel: %v", err)
						return
					}
					// Wait for connection to be ready
					time.Sleep(time.Second)
				}
			}

			go func() {
				dgvoice.PlayAudioFile(currentVC, song.Filename, done)
				<-done
				musicQueue.SetPlaying(false)
				musicQueue.SetCurrentSong(nil)
				if !musicQueue.IsPaused() {
					go playNextSong(s, guildID, channelID)
				}
			}()

			select {
			case <-musicQueue.Done:
				close(done)
				return
			}
		}
		time.Sleep(time.Second)
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
