package blackfire

import (
	"io"
	"log"
	"os"
	"strconv"

	"github.com/rs/zerolog"
)

func NewLogger(path string, level int) zerolog.Logger {
	return zerolog.New(logWriter(path)).Level(logLevel(level)).With().Timestamp().Logger()
}

func NewLoggerFromEnvVars() zerolog.Logger {
	level := 1
	if v := os.Getenv("BLACKFIRE_LOG_LEVEL"); v != "" {
		level, _ = strconv.Atoi(v)
	}
	path := ""
	if v := os.Getenv("BLACKFIRE_LOG_FILE"); v != "" {
		path = v
	}
	return zerolog.New(logWriter(path)).Level(logLevel(level)).With().Timestamp().Logger()
}

func logLevel(level int) zerolog.Level {
	if level < 1 {
		level = 1
	}
	if level > 4 {
		level = 4
	}
	var levels = map[int]zerolog.Level{
		1: zerolog.ErrorLevel,
		2: zerolog.WarnLevel,
		3: zerolog.InfoLevel,
		4: zerolog.DebugLevel,
	}
	return levels[level]
}

func logWriter(path string) io.Writer {
	if path == "" || path == "stderr" {
		return os.Stderr
	}
	if path == "stdout" {
		return os.Stdout
	}
	writer, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatalf("could not open log file at %s: %v", path, err)
	}
	return writer
}
