package hybrid

import (
	"fmt"
	"log"
	"os"
)

// LogLevel represents the logging level
type LogLevel int

const (
	// LogLevelNone disables all logging
	LogLevelNone LogLevel = iota

	// LogLevelFatal enables fatal logging
	LogLevelFatal

	// LogLevelError enables error logging
	LogLevelError
	// LogLevelInfo enables info and error logging
	LogLevelInfo
	// LogLevelDebug enables all logging
	LogLevelDebug
)

var currentLogLevel = LogLevelInfo

var (
	debugLogger *log.Logger
	infoLogger  *log.Logger
	errorLogger *log.Logger
	fatalLogger *log.Logger
)

func init() {
	debugLogger = log.New(os.Stdout, "[DEBUG] ", log.Ldate|log.Ltime|log.Lshortfile)
	infoLogger = log.New(os.Stdout, "[Info] ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)
	fatalLogger = log.New(os.Stderr, "[FATAL] ", log.Ldate|log.Ltime|log.Lshortfile)
}

// Debug logs debug information
func Debug(format string, v ...interface{}) {
	if currentLogLevel >= LogLevelDebug {
		debugLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Error logs error information
func Error(format string, v ...interface{}) {
	if currentLogLevel >= LogLevelError {
		errorLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Error logs error information
func Info(format string, v ...interface{}) {
	if currentLogLevel >= LogLevelInfo {
		infoLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Fatal logs fatal information
func Fatal(format string, v ...interface{}) {
	if currentLogLevel >= LogLevelFatal {
		fatalLogger.Fatal(2, fmt.Sprintf(format, v...))
	}
}
