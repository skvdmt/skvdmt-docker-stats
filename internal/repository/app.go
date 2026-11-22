package repository

import (
	"context"
	"sync/atomic"

	"github.com/skvdmt/skvdmt-docker-stats/internal/model"
)

const (
	SOURCES = 0
)

// App Репозиторный слой.
type App struct {
	// Контекст приложения.
	ctx context.Context
	// Счетчик готовых ресурсов.
	ready atomic.Int32
	// Docker
	*Docker
}

// NewApp Конструктор.
func NewApp(ctx context.Context) (*App, error) {
	model.Log.Info.Info("repository layer creating")
	d, err := NewDocker(ctx)
	if err != nil {
		return nil, err
	}
	a := &App{
		ctx:    ctx,
		Docker: d,
	}
	return a, nil
}

// Start Запуск.
func (a *App) Start() error {
	model.Log.Info.Info("repository layer starting")
	return nil
}

// Ready Готовность.
func (a *App) Ready() bool {
	return a.ready.Load() == SOURCES
}

// Stop Остановка.
func (a *App) Stop() error {
	close(a.Docker.stats)
	if err := a.Docker.cli.Close(); err != nil {
		return err
	}
	model.Log.Info.Info("docker client stopped")
	defer model.Log.Info.Info("repository layer stopped")
	return nil
}
