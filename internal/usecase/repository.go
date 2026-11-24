package usecase

import (
	"context"

	"github.com/moby/moby/api/types/container"
	"github.com/skvdmt/skvdmt-docker-stats/internal/entities"
)

// Repository Интерфейс репозитория.
type Repository interface {
	// Запуск.
	Start() error
	// Готовность.
	Ready() bool
	// Остановка.
	Stop() error
	// Канал для получения статистики Docker.
	DockerStats() <-chan *container.StatsResponse
	// Имена контейнеров Docker.
	DockerContainerNames() ([]*entities.ContainerInfo, error)
	// Создание обработчика статистики контейнера Docker с именем name.
	DockerStatHandler(ctx context.Context, name string)
}
