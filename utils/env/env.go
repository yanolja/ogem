package env

import (
	"log"
	"os"
	"strconv"
)

var logFatalf = log.Fatalf

func RequiredStringVariable(name string) string {
	if !HasEnv(name) {
		logFatalf("Environment variable (%s) is required but does not exist.", name)
	}
	return os.Getenv(name)
}

func OptionalStringVariable(name string, defaultValue string) string {
	if !HasEnv(name) {
		return defaultValue
	}
	return os.Getenv(name)
}

func OptionalIntVariable(name string, defaultValue int) int {
	if !HasEnv(name) {
		return defaultValue
	}
	value := os.Getenv(name)
	intValue, err := strconv.Atoi(value)
	if err != nil {
		logFatalf("Environment variable (%s) is not a valid int.", name)
	}
	return intValue
}

func OptionalBoolVariable(name string, defaultValue bool) bool {
	if !HasEnv(name) {
		return defaultValue
	}
	value := os.Getenv(name)
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		logFatalf("Environment variable (%s) is not a valid bool.", name)
	}
	return boolValue
}

func HasEnv(name string) bool {
	_, ok := os.LookupEnv(name)
	return ok
}
