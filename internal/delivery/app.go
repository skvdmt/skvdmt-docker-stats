package delivery

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/skvdmt/skvdmt-docker-stats/internal/model"
	"github.com/skvdmt/skvdmt-docker-stats/internal/usecase"
)

const (
	SOURCES = 1
)

// App Транспортный слой.
type App struct {
	// Контекст приложения.
	ctx context.Context
	// Сервисный слой.
	usecase Usecase
	// Счетчик готовых ресурсов.
	ready atomic.Int32
	// Менеджер клиентов.
	cm *ClientManager
	// Счетчик запущенных процессов.
	wg sync.WaitGroup
}

// NewApp Конструктор.
func NewApp(ctx context.Context) (*App, error) {
	model.Log.Info.Info("delivery layer creating")
	u, err := usecase.NewApp(ctx)
	if err != nil {
		return nil, err
	}
	return &App{
		ctx:     ctx,
		usecase: u,
		cm:      NewClientManager(),
	}, nil
}

// Start Запуск.
func (a *App) Start() error {
	if err := a.usecase.Start(); err != nil {
		return err
	}

	model.Log.Info.Info("delivery layer starting")

	a.wg.Add(1)
	go a.eventsHandler()

	return nil
}

// Ready Готовность.
func (a *App) Ready() bool {
	return a.ready.Load() == SOURCES && a.usecase.Ready()
}

// Stop Остановка.
func (a *App) Stop() error {
	defer model.Log.Info.Info("delivery layer stopped")
	if err := a.usecase.Stop(); err != nil {
		return err
	}
	a.wg.Wait()
	return nil
}

// DockerStats Статистика Docker.
func (a *App) DockerStats(w http.ResponseWriter, r *http.Request) {
	f, o := w.(http.Flusher)
	if !o {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	c := NewClient()
	a.cm.Register(c)
	defer a.cm.Unregister(c)

	msgs, err := a.usecase.DockerStats()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Отправка полной статистики при подключении клиента.
	for _, msg := range msgs {
		fmt.Fprintf(w, "data: %s\n\n", msg)
	}
	f.Flush()

	for {
		select {
		case <-r.Context().Done():
			model.Log.Info.Info("client disconnected by ctx.")
			return
		case msg, o := <-c.send:
			if !o {
				model.Log.Info.Info("client disconnected.")
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			f.Flush()
		case <-a.ctx.Done():
			model.Log.Info.Info("close connection by server shutdown.")
			fmt.Fprintf(w, "data: %s\n\n", `{"event":"closed"}`)
			f.Flush()
			return
		}
	}
}

// eventsHandler Обработчик событий.
func (a *App) eventsHandler() {
	model.Log.Info.Info("delivery events listener starting")
	defer func() {
		model.Log.Info.Info("delivery events listener stopped")
		a.cm.Close()
		a.wg.Done()
	}()
	a.ready.Add(1)
	for e := range a.usecase.DockerStatsChanges() {
		a.cm.Broadcast(e)
	}
}
