package internal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/skvdmt/skvdmt-docker-stats/internal/delivery"
	"github.com/skvdmt/skvdmt-docker-stats/internal/model"
)

var (
	TIMEOUT               = 3_000 * time.Millisecond
	SOURCES               = 2
	HEALTH_CHECK_INTERVAL = 100 * time.Millisecond
	HEALTH_CHECK_TIMEOUT  = 3_000 * time.Millisecond
	HTTP_SHUTDOWN_TIMEOUT = 12 * time.Second
	READ_TIMEOUT          = 5 * time.Second
)

// App Основная структура приложения.
type App struct {
	// Контекст приложения.
	ctx context.Context
	// Отмена контекста.
	cancel context.CancelFunc
	// Транспортный слой.
	delivery Delivery
	// Ошибки приложения.
	errors []error
	// Канал сигналов прерывания.
	interrupt chan os.Signal
	// Приложение уже останавливается.
	stopping bool
	// Счетчик ресурсов.
	wg sync.WaitGroup
	// Роутер.
	router *http.ServeMux
	// Сервер.
	sever *http.Server
	// Счетчик готовых ресурсов.
	ready int
}

// NewApp Конструктор.
func NewApp() (*App, error) {
	model.Log.Info.Info(fmt.Sprintf("app %s creating", model.APP_NAME))
	var err error
	model.Conf, err = model.NewConfig()
	model.MinorErrors = make(chan error)
	model.CommonErrors = make(chan error)
	model.CriticalErrors = make(chan error)
	a := &App{
		wg:        sync.WaitGroup{},
		interrupt: make(chan os.Signal),
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.router = http.NewServeMux()
	a.sever = &http.Server{
		Addr:        fmt.Sprintf(":%d", model.Conf.Server.Port),
		Handler:     a.router,
		ReadTimeout: READ_TIMEOUT,
	}
	a.delivery, err = delivery.NewApp(a.ctx)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// Start Запуск.
func (a *App) Start() []error {
	model.Log.Info.Info(fmt.Sprintf("app %s starting", model.APP_NAME))
	a.routing()

	a.wg.Add(1)
	go a.errorHandler()

	a.wg.Add(1)
	go func() {
		defer func() {
			model.Log.Info.Info("http server stopped")
			a.wg.Done()
		}()
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", model.Conf.Server.Port))
		if err != nil {
			model.CommonErrors <- err
			return
		}
		a.ready += 1
		if err := a.healthCheckLock(); err != nil {
			model.CommonErrors <- err
			return
		}
		model.Log.Info.Info(fmt.Sprintf("http server started on %d port", model.Conf.Server.Port))
		if err := a.sever.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			model.CommonErrors <- err
			return
		}
	}()

	if err := a.delivery.Start(); err != nil {
		model.CommonErrors <- err
	}

	return a.interruptHandler()
}

// healthCheckLock Блокировка до готовности.
func (a *App) healthCheckLock() error {
	s := time.Now()
	for {
		if a.Ready() {
			break
		}
		if time.Since(s) > HEALTH_CHECK_TIMEOUT {
			return fmt.Errorf("health check timeout")
		}
		time.Sleep(HEALTH_CHECK_INTERVAL)
	}
	return nil
}

// Ready Готовность.
func (a *App) Ready() bool {
	return a.ready == SOURCES && a.delivery.Ready()
}

// ErrorHandler Обработчик ошибок.
func (a *App) errorHandler() {
	model.Log.Info.Info("error handler starting")
	defer func() {
		model.Log.Info.Info("error handler stopped")
		a.wg.Done()
	}()
	a.ready += 1
	for {
		select {
		case err := <-model.MinorErrors:
			if err == nil {
				return
			}
			if errors.Is(err, context.Canceled) {
				continue
			}
			a.errors = append(a.errors, err)
		case err := <-model.CommonErrors:
			if err == nil {
				return
			}
			if errors.Is(err, context.Canceled) {
				continue
			}
			a.errors = append(a.errors, err)
			if !a.stopping {
				a.interrupt <- syscall.SIGTERM
			}
		case err := <-model.MinorErrors:
			if err == nil {
				return
			}
			a.errors = append(a.errors, err)
			if !a.stopping {
				a.interrupt <- syscall.SIGTERM
			}
		}
	}
}

// InterruptHandler Обработчик сигналов прерывания.
func (a *App) interruptHandler() []error {
	model.Log.Info.Info("waiting interrupt signal")
	signal.Notify(a.interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-a.interrupt
	model.Log.Info.Info("received interrupt signal")
	a.stop()
	close(model.MinorErrors)
	close(model.CommonErrors)
	close(model.CriticalErrors)
	a.wg.Wait()
	return a.errors
}

// Stop Остановка.
func (a *App) stop() {
	defer func() {
		model.Log.Info.Info(fmt.Sprintf("app %s stopped", model.APP_NAME))
	}()
	a.stopping = true
	a.cancel()
	model.Log.Info.Info("context canceled")
	if err := a.healthCheckLock(); err != nil {
		model.CommonErrors <- err
	}
	ctx, cancel := context.WithTimeout(context.Background(), HTTP_SHUTDOWN_TIMEOUT)
	if err := a.sever.Shutdown(ctx); err != nil {
		model.CommonErrors <- err
	}
	cancel()
	if err := a.delivery.Stop(); err != nil {
		model.CommonErrors <- err
	}
	close(a.interrupt)
}

// routing Установка маршрутов.
func (a *App) routing() {
	a.router.HandleFunc("GET /stats", a.delivery.DockerStats)
}
