package blackfire

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

var currentLogLevel int
var currentLogPath string

const defaultLogLevel = 3
const defaultLogfile = ""

var logLevelMappings = map[int]zerolog.Level{
	1: zerolog.ErrorLevel,
	2: zerolog.WarnLevel,
	3: zerolog.InfoLevel,
	4: zerolog.DebugLevel,
}

func init() {
	setLogFileInternal(defaultLogfile)
	setLogLevelInternal(defaultLogLevel)
}

func setLogLevelInternal(level int) {
	if level < 1 {
		level = 1
	}
	if level > 4 {
		level = 4
	}

	Log = Log.Level(logLevelMappings[level])
	currentLogLevel = level
}

func setLogFileInternal(filePath string) (err error) {
	var writer io.Writer
	if filePath == "" || strings.EqualFold(filePath, "stderr") {
		writer = os.Stderr
	} else if strings.EqualFold(filePath, "stdout") {
		writer = os.Stdout
	} else {
		writer, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0664)
		if err != nil {
			Log.Error().Msgf("Could not open log file at %s: %v", filePath, err)
			return
		}
	}
	Log = Log.Output(writer)
	currentLogPath = filePath
	return
}

// Note: Log config functions must be idempotent because they get called many
// times in the configuration code.

func setLogLevel(level int) error {
	Log.Debug().Msgf("Blackfire: Change log level to %d", level)
	if currentLogLevel != level {
		_, ok := logLevelMappings[level]
		if !ok {
			return fmt.Errorf("Blackfire: %d: Invalid log level (must be 1-4)", level)
		}
		setLogLevelInternal(level)
	}
	return nil
}

func setLogFile(filePath string) (err error) {
	Log.Debug().Msgf("Blackfire: Change log file to %s", filePath)
	if currentLogPath != filePath {
		return setLogFileInternal(filePath)
	}
	return
}
