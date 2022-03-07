package postgres

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"

	"42stellar.org/webhooks/internal/valuable"
)

// storage is the struct contains client and config
// Run is made from external caller at begins programs
type storage struct {
	client *sql.DB
	config *config
}

// config is the struct contains config for connect client
// Run is made from internal caller
type config struct {
	DatabaseURL valuable.Valuable
	TableName   string
	DataField   string
}

// NewStorage is the function for create new Postgres client storage
// Run is made from external caller at begins programs
// @param config contains config define in the webhooks yaml file
// @return PostgresStorage the struct contains client connected and config
// @error an error if the the client is not initialized succesfully
func NewStorage(configRaw map[string]interface{}) (*storage, error) {
	var err error

	newClient := storage{
		config: &config{},
	}

	if err := valuable.Decode(configRaw, &newClient.config); err != nil {
		return nil, err
	}

	if newClient.client, err = sql.Open("postgres", newClient.config.DatabaseURL.First()); err != nil {
		return nil, err
	}

	return &newClient, nil
}

// Name is the function for identified if the storage config is define in the webhooks
// Run is made from external caller
func (c storage) Name() string {
	return "postgres"
}

// Push is the function for push data in the storage
// A run is made from external caller
// @param value that will be pushed
// @return an error if the push failed
func (c storage) Push(value interface{}) error {
	request := fmt.Sprintf("INSERT INTO %s(%s) VALUES ('%s')", c.config.TableName, c.config.DataField, value)
	if _, err := c.client.Query(request); err != nil {
		return err
	}

	return nil
}