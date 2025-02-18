package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func getDotEnv(key string) string {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	return os.Getenv(key)
}
