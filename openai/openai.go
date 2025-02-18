package openai

import (
	"context"
	"fmt"
	"log"

	openai "github.com/sashabaranov/go-openai"
	"github.com/svnfrs/ok-bot/env"
)

func AskChatGPT(message string) string {
	client := openai.NewClient(env.GetEnv("OPEN_AI_KEY"))
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4o,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: message,
				},
			},
		},
	)

	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return ""
	}

	response := resp.Choices[0].Message.Content

	log.Println(response)

	return response
}
