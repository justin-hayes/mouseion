package testutil

import "os"

const defaultTestDatabaseURL = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"

func externalDatabaseURL() (string, bool) {
	if databaseURL := os.Getenv("MOUSEION_TEST_DATABASE_URL"); databaseURL != "" {
		return databaseURL, true
	}
	return defaultTestDatabaseURL, false
}
