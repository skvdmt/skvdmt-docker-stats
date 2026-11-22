package model

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	LOG_DIR_DEV  = "./logs"
	LOG_DIR_PROD = "/var/log"
	LOG_DIR_PERM = 0o755

	LOG_ERROR_FILENAME = "error.log"
	LOG_ERROR_FLAGS    = os.O_APPEND | os.O_WRONLY | os.O_CREATE
	LOG_ERROR_PERM     = 0o644
)

// Log Глобальный экземпляр ведения журнала.
var Log *Logger

// Logger Ведение журнала.
type Logger struct {
	Info      *slog.Logger
	Error     *slog.Logger
	errorFile *os.File
}

// NewLogger Конструктор.
func NewLogger() (*Logger, error) {
	ld, err := logDir()
	if err != nil {
		return nil, err
	}
	ef, err := os.OpenFile(
		filepath.Join(ld,
			LOG_ERROR_FILENAME),
		LOG_ERROR_FLAGS, LOG_ERROR_PERM,
	)
	if err != nil {
		return nil, err
	}
	return &Logger{
		Info:      slog.New(slog.NewTextHandler(os.Stdout, nil)),
		Error:     slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stderr, ef), nil)),
		errorFile: ef,
	}, nil
}

// Close Закрытие ведения журнала.
func (l *Logger) Close() error {
	return l.errorFile.Close()
}

// logDir Получение директории журнала.
func logDir() (logDir string, err error) {
	defer func() {
		err = os.MkdirAll(logDir, LOG_DIR_PERM)
	}()
	if os.Getenv(MODE_ENV) == MODE_DEV {
		return LOG_DIR_DEV, nil
	}
	return filepath.Join(LOG_DIR_PROD, APP_NAME), nil
}
