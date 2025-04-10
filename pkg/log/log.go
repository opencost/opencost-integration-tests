package log

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func InitLogging(showLogLevelSetMessage bool) {
	zerolog.TimeFieldFormat = time.RFC3339Nano

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func Error(msg string) {
	log.Error().Msg(msg)
}

func Errorf(format string, a ...interface{}) {
	log.Error().Msgf(format, a...)
}

func Warn(msg string) {
	log.Warn().Msg(msg)
}

func Warnf(format string, a ...interface{}) {
	log.Warn().Msgf(format, a...)
}

func Info(msg string) {
	log.Info().Msg(msg)
}

func Infof(format string, a ...interface{}) {
	log.Info().Msgf(format, a...)
}

func Debug(msg string) {
	log.Debug().Msg(msg)
}

func Debugf(format string, a ...interface{}) {
	log.Debug().Msgf(format, a...)
}

func Trace(msg string) {
	log.Trace().Msg(msg)
}

func Tracef(format string, a ...interface{}) {
	log.Trace().Msgf(format, a...)
}

func Fatal(msg string) {
	log.Fatal().Msg(msg)
}

func Fatalf(format string, a ...interface{}) {
	log.Fatal().Msgf(format, a...)
}
