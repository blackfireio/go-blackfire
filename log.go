package blackfire

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

var logLevelMappings = map[int]zerolog.Level{
	1: zerolog.ErrorLevel,
	2: zerolog.WarnLevel,
	3: zerolog.InfoLevel,
	4: zerolog.DebugLevel,
}

func init() {
	setLogFile("stderr")
	setLogLevel(1)
}

func setLogLevel(level int) {
	if level < 1 {
		level = 1
	}
	if level > 4 {
		level = 4
	}

	Log = Log.Level(logLevelMappings[level])
}

func setLogFile(filePath string) error {
	var writer io.Writer
	if filePath == "" || strings.EqualFold(filePath, "stderr") {
		writer = os.Stderr
	} else if strings.EqualFold(filePath, "stdout") {
		writer = os.Stdout
	} else {
		var err error
		writer, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0664)
		if err != nil {
			return fmt.Errorf("could not open log file at %s: %v", filePath, err)
		}
	}
	Log = Log.Output(writer)
	return nil
}
