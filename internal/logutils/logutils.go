package logutils

import (
	"fmt"
	"log"
	"runtime"
	"strings"
)

var Log *Logger

type Logger struct {
	level  LogLevel
	err    error
	fields map[string]any
}

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func InitLogger(level string) {
	parsedLevel, err := parseLogLevel(level)
	if err != nil {
		log.Printf("Invalid log level '%s', defaulting to 'info'", level)
		parsedLevel = LevelInfo
	}
	Log = &Logger{level: parsedLevel}
	Log.Infof("Log level set to %v", parsedLevel)
}

func parseLogLevel(level string) (LogLevel, error) {
	switch strings.ToLower(level) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("invalid log level: %s", level)
	}
}

func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		level:  l.level,
		err:    err,
		fields: l.fields,
	}
}

func (l *Logger) WithField(key string, value any) *Logger {
	newFields := copyFields(l.fields)
	newFields[key] = value
	return &Logger{
		level:  l.level,
		err:    l.err,
		fields: newFields,
	}
}

func (l *Logger) WithFields(fields map[string]any) *Logger {
	newFields := copyFields(l.fields)
	for k, v := range fields {
		newFields[k] = v
	}
	return &Logger{
		level:  l.level,
		err:    l.err,
		fields: newFields,
	}
}

func copyFields(fields map[string]any) map[string]any {
	newFields := make(map[string]any)
	for k, v := range fields {
		newFields[k] = v
	}
	return newFields
}

func (l *Logger) formatFields() string {
	if len(l.fields) == 0 {
		return ""
	}
	var sb strings.Builder
	for k, v := range l.fields {
		sb.WriteString(fmt.Sprintf("%s=%v ", k, v))
	}
	return sb.String()
}

func (l *Logger) traceInfo() string {
	_, file, line, ok := runtime.Caller(2)
	if ok {
		if l.level == LevelDebug {
			return fmt.Sprintf("%s:%d ", file, line)
		}
		return fmt.Sprintf("%s:%d ", file, line)
	}
	return ""
}

func (l *Logger) Debugf(format string, args ...any) {
	if l.level <= LevelDebug {
		trace := l.traceInfo()
		if len(l.fields) > 0 {
			log.Printf("[DEBUG] "+trace+l.formatFields()+format, args...)
		} else {
			log.Printf("[DEBUG] "+trace+format, args...)
		}
	}
}

func (l *Logger) Debug(message string) {
	if l.level <= LevelDebug {
		trace := l.traceInfo()
		log.Printf("[DEBUG] %s%s%s", trace, l.formatFields(), message)
	}
}

func (l *Logger) Infof(format string, args ...any) {
	if l.level <= LevelInfo {
		trace := ""
		if l.level == LevelDebug {
			trace = l.traceInfo()
		}
		if len(l.fields) > 0 {
			log.Printf("[INFO] "+trace+l.formatFields()+format, args...)
		} else {
			log.Printf("[INFO] "+trace+format, args...)
		}
	}
}

func (l *Logger) Info(message string) {
	if l.level <= LevelInfo {
		trace := ""
		if l.level == LevelDebug {
			trace = l.traceInfo()
		}
		log.Printf("[INFO] %s%s%s", trace, l.formatFields(), message)
	}
}

func (l *Logger) Warnf(format string, args ...any) {
	if l.level <= LevelWarn {
		trace := l.traceInfo()
		if len(l.fields) > 0 {
			log.Printf("[WARN] "+trace+l.formatFields()+format, args...)
		} else {
			log.Printf("[WARN] "+trace+format, args...)
		}
	}
}

func (l *Logger) Warn(message string) {
	if l.level <= LevelWarn {
		trace := l.traceInfo()
		log.Printf("[WARN] %s%s%s", trace, l.formatFields(), message)
	}
}

func (l *Logger) Errorf(format string, args ...any) {
	if l.level <= LevelError {
		trace := l.traceInfo()
		if l.err != nil {
			if len(l.fields) > 0 {
				log.Printf("[ERROR] "+trace+l.formatFields()+format+": %v", append(args, l.err)...)
			} else {
				log.Printf("[ERROR] "+trace+format+": %v", append(args, l.err)...)
			}
		} else {
			if len(l.fields) > 0 {
				log.Printf("[ERROR] "+trace+l.formatFields()+format, args...)
			} else {
				log.Printf("[ERROR] "+trace+format, args...)
			}
		}
	}
}

func (l *Logger) Error(message string) {
	if l.level <= LevelError {
		trace := l.traceInfo()
		if l.err != nil {
			log.Printf("[ERROR] %s%s%s: %v", trace, l.formatFields(), message, l.err)
		} else {
			log.Printf("[ERROR] %s%s%s", trace, l.formatFields(), message)
		}
	}
}

func (*Logger) Fatal(format string, args ...any) {
	_, file, line, ok := runtime.Caller(1)
	trace := ""
	if ok {
		trace = fmt.Sprintf("%s:%d ", file, line)
	}
	log.Printf("[FATAL] "+trace+format, args...)
	log.Fatal()
}
