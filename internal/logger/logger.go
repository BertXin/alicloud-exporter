package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger wraps logrus.Logger with additional functionality
type Logger struct {
	*logrus.Logger
}

// New creates a new logger instance
func New(level, format string) *Logger {
	logger := logrus.New()
	
	// Set log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)
	
	// Set log format
	switch format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	default:
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	}
	
	// Set output to stdout
	logger.SetOutput(os.Stdout)
	
	return &Logger{Logger: logger}
}

// WithFields creates a new logger entry with fields
func (l *Logger) WithFields(fields map[string]interface{}) *logrus.Entry {
	return l.Logger.WithFields(fields)
}

// WithField creates a new logger entry with a single field
func (l *Logger) WithField(key string, value interface{}) *logrus.Entry {
	return l.Logger.WithField(key, value)
}

// WithError creates a new logger entry with an error field
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// WithService creates a new logger entry with service field
func (l *Logger) WithService(service string) *logrus.Entry {
	return l.Logger.WithField("service", service)
}

// WithMetric creates a new logger entry with metric field
func (l *Logger) WithMetric(metric string) *logrus.Entry {
	return l.Logger.WithField("metric", metric)
}

// WithInstance creates a new logger entry with instance field
func (l *Logger) WithInstance(instance string) *logrus.Entry {
	return l.Logger.WithField("instance", instance)
}