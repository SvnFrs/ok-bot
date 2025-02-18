package main

import (
	"log"
	"sync"

	"github.com/bwmarrin/discordgo"
)

var (
	Token = getDotEnv("DISCORD_BOT_KEY")
)

var mu sync.Mutex

func startBot() {
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
			response := chatGPT(question)

			// edit the original response with the actual content
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &response,
			})
			if err != nil {
				log.Printf("error editing response: %v", err)
			}
		}()
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
