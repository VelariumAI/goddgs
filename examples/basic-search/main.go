package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/velariumai/goddgs"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := goddgs.LoadConfigFromEnv()
	engine, err := goddgs.NewDefaultEngineFromConfig(cfg)
	if err != nil {
		return err
	}

	resp, err := engine.Search(context.Background(), goddgs.SearchRequest{
		Query:      "golang concurrency patterns",
		MaxResults: 5,
		Region:     "us-en",
	})
	if err != nil {
		return err
	}

	out, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(out))
	return nil
}
