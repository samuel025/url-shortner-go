package env

import (
	"os"
	"strconv"
	"log"
	"github.com/joho/godotenv"
)

func GetEnvString(key, defaultValue string) string {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultValue
}

func GetEnvInt(key string, defaultValue int) int {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}

	return defaultValue
}
