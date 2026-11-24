package internal

import "net/http"

// Deliver Интерфейс транспортного слоя.
type Delivery interface {
	// Запуск.
	Start() error
	// Готовность.
	Ready() bool
	// Остановка.
	Stop() error
	// Статистика Docker.
	DockerStats(http.ResponseWriter, *http.Request)
}
