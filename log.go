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
	Log = Log.Level(logLevelMappings[level])
	currentLogLevel = level
}

func setLogFileInternal(filePath string) (err error) {
	var writer io.Writer
	if filePath == "" || strings.EqualFold(filePath, "stderr") {
		writer = os.Stderr
	} else {
		writer, err = os.Open(filePath)
		if err != nil {
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
	Log.Debug().Msgf("Blackfire: Change log level to %v", level)
	if currentLogLevel != level {
		_, ok := logLevelMappings[level]
		if !ok {
			return fmt.Errorf("Blackfire: %v: Invalid log level (must be 1-4)", level)
		}
		setLogLevelInternal(level)
	}
	return nil
}

func setLogFile(filePath string) (err error) {
	Log.Debug().Msgf("Blackfire: Change log file to %v", filePath)
	if currentLogPath != filePath {
		return setLogFileInternal(filePath)
	}
	return
}
