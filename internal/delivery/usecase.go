package delivery

import "encoding/json"

// Usecase Интерфейс сервисного слоя.
type Usecase interface {
	// Запуск.
	Start() error
	// Готовность.
	Ready() bool
	// Остановка.
	Stop() error
	// Полная статистика Docker.
	DockerStats() ([]json.RawMessage, error)
	// Изменения статистики Docker.
	DockerStatsChanges() <-chan json.RawMessage
}
