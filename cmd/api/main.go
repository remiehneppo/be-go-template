package main

import (
	"fmt"
	"os"

	"github.com/remihneppo/be-go-template/internal/config"
	"github.com/remihneppo/be-go-template/internal/platform/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	log, closeLog, err := logger.New(logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		FilePath:   cfg.Log.FilePath,
		ToTerminal: cfg.Log.ToTerminal,
		ToFile:     cfg.Log.ToFile,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := closeLog(); err != nil {
			fmt.Fprintf(os.Stderr, "close logger: %v\n", err)
		}
	}()

	log.Info("api bootstrap ready", logger.String("app", cfg.App.Name), logger.String("env", cfg.App.Env), logger.String("addr", cfg.HTTP.Addr))
}
