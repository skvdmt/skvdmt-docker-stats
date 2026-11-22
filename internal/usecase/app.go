package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/skvdmt/skvdmt-docker-stats/internal/entities"
	"github.com/skvdmt/skvdmt-docker-stats/internal/model"
	"github.com/skvdmt/skvdmt-docker-stats/internal/repository"
)

const (
	SOURCES = 2
)

var (
	ERR_UNHANDLED_CONTAINER          = errors.New("unhandled container")
	ERR_NO_CHANGES                   = errors.New("no changes")
	REFRESH_CONTAINER_NAMES_INTERVAL = 5_000 * time.Millisecond
)

// App Сервисный слой.
type App struct {
	// Контекст приложения.
	ctx context.Context
	// Репозиторный слой.
	repository Repository
	// Счетчик готовых ресурсов.
	ready atomic.Int32
	// Сигнал выхода из обработчика обнавления имен контейнеров.
	exitRCNH chan struct{}
	// Счетчик запущенных ресурсов.
	wg sync.WaitGroup
	// Обработкики статистики Docker. Ключ - имя контейнера.
	handlers map[string]DockerStatHandler
	// Мьютекс.
	mu sync.Mutex
	// Изменения.
	changes chan json.RawMessage
}

// DockerHandler Обработчик статистики Docker.
type DockerStatHandler struct {
	ctx    context.Context
	cancel context.CancelFunc
	stats  *entities.DockerStats
}

// NewApp Конструктор.
func NewApp(ctx context.Context) (*App, error) {
	model.Log.Info.Info("usecase layer creating")
	a := &App{
		ctx:      ctx,
		exitRCNH: make(chan struct{}),
		wg:       sync.WaitGroup{},
		handlers: make(map[string]DockerStatHandler),
		changes:  make(chan json.RawMessage),
	}
	var err error
	a.repository, err = repository.NewApp(ctx)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// Start Запуск.
func (a *App) Start() error {
	if err := a.repository.Start(); err != nil {
		return err
	}
	a.refreshContainerNames()
	a.ready.Add(1)

	a.wg.Add(1)
	go a.dockerStatsHandler()

	a.wg.Add(1)
	go a.refreshContainerNamesHandler()

	model.Log.Info.Info("usecase layer starting")
	return nil
}

// Ready Готовность.
func (a *App) Ready() bool {
	return a.ready.Load() == SOURCES && a.repository.Ready()
}

// Stop Остановка.
func (a *App) Stop() error {
	defer model.Log.Info.Info("usecase layer stopped")
	if err := a.repository.Stop(); err != nil {
		return err
	}
	close(a.exitRCNH)
	a.wg.Wait()
	close(a.changes)
	return nil
}

// DockerStats Полная статистика Docker.
func (a *App) DockerStats() ([]json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	raws := []json.RawMessage{}
	for _, h := range a.handlers {
		raw, err := json.Marshal(h.stats)
		if err != nil {
			return nil, err
		}
		raws = append(raws, raw)
	}
	return raws, nil
}

// DockerStatsChanges Изменения статистики Docker.
func (a *App) DockerStatsChanges() <-chan json.RawMessage {
	return a.changes
}

// dockerStatsHandler Обработчик статистики Docker.
func (a *App) dockerStatsHandler() {
	defer func() {
		model.Log.Info.Info("docker stats handler stopped")
		a.wg.Done()
	}()
	model.Log.Info.Info("docker stats handler starting")
	a.ready.Add(1)
	for s := range a.repository.DockerStats() {
		if !a.valid(s) {
			continue
		}
		u, err := a.updated(a.format(s))
		if err != nil {
			if !errors.Is(err, ERR_UNHANDLED_CONTAINER) &&
				!errors.Is(err, ERR_NO_CHANGES) {
				model.MinorErrors <- err
			}
			continue
		}
		raw, _ := json.Marshal(u)
		a.changes <- raw
	}
}

// refreshContainerNamesHandler Обработчик обновления имен контейнеров Docker.
func (a *App) refreshContainerNamesHandler() {
	defer func() {
		model.Log.Info.Info("refresh container names handler stopped")
		a.wg.Done()
	}()
	model.Log.Info.Info("refresh container names handler starting")
	t := time.NewTicker(REFRESH_CONTAINER_NAMES_INTERVAL)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			a.refreshContainerNames()
		case <-a.exitRCNH:
			return
		}
	}
}

// refreshContainerNames Обновление имен контейнеров Docker.
// В методе применяется фильтр игнорирующий некоторые имена.
func (a *App) refreshContainerNames() {
	model.Log.Info.Info("refresh container names")
	ns, err := a.repository.DockerContainerNames()
	if err != nil {
		model.CommonErrors <- err
		return
	}
	f := slices.DeleteFunc(ns, func(n string) bool {
		return strings.HasPrefix(n, "buildx_") ||
			strings.Contains(n, "test") ||
			n == "redis-sse-server" ||
			n == "migration" ||
			n == "certbot"
	})
	a.containerNamesHandle(f)
}

// containerNamesHandle Обработка имен контейнеров.
func (a *App) containerNamesHandle(names []string) {
	// Добавление новых контейнеров.
	for _, n := range names {
		a.mu.Lock()
		_, o := a.handlers[n]
		if !o {
			ctx, cancel := context.WithCancel(a.ctx)
			a.handlers[n] = DockerStatHandler{
				ctx:    ctx,
				cancel: cancel,
				stats: &entities.DockerStats{
					Name: n,
				},
			}
			a.wg.Add(1)
			go func(ctx context.Context, cancel context.CancelFunc, n string) {
				defer func() {
					model.Log.Info.Info(fmt.Sprintf("container name handler %s stopped", n))
					cancel()
					a.wg.Done()
				}()
				model.Log.Info.Info(fmt.Sprintf("container name handler %s creating", n))
				a.repository.DockerStatHandler(ctx, n)
			}(ctx, cancel, n)
		}
		a.mu.Unlock()
	}
	// Удаление старых обработчиков.
	a.mu.Lock()
	for n, h := range a.handlers {
		if !slices.Contains(names, n) {
			h.cancel()
			delete(a.handlers, n)
			raw, _ := json.Marshal(map[string]string{"remove": n})
			select {
			case a.changes <- raw:
			case <-a.ctx.Done():
				return
			}
		}
	}
	a.mu.Unlock()
}

// valid Проверка корректности статистики.
func (a *App) valid(s *container.StatsResponse) bool {
	return s != nil && len(s.ID) >= 12
}

// updated Обновления.
func (a *App) updated(upd *entities.DockerStats) (map[string]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, o := a.handlers[upd.Name]
	if !o {
		return nil, fmt.Errorf("%w: %s", ERR_UNHANDLED_CONTAINER, upd.Name)
	}
	old := a.handlers[upd.Name].stats
	emp := map[string]string{
		"name":         upd.Name,
		"container_id": upd.Id,
	}
	c := false
	if *old == *upd {
		return nil, ERR_NO_CHANGES
	}
	if old.CPUPercent != upd.CPUPercent {
		emp["cpu_percent"] = upd.CPUPercent
		c = true
	}
	if old.MemUsage != upd.MemUsage {
		emp["mem_usage"] = upd.MemUsage
		c = true
	}
	if old.MemLimit != upd.MemLimit {
		emp["mem_limit"] = upd.MemLimit
		c = true
	}
	if old.MemPercent != upd.MemPercent {
		emp["mem_percent"] = upd.MemPercent
		c = true
	}
	if old.NetRx != upd.NetRx {
		emp["net_rx"] = upd.NetRx
		c = true
	}
	if old.NetTx != upd.NetTx {
		emp["net_tx"] = upd.NetTx
		c = true
	}
	if old.BlockRead != upd.BlockRead {
		emp["block_read"] = upd.BlockRead
		c = true
	}
	if old.BlockWrite != upd.BlockWrite {
		emp["block_write"] = upd.BlockWrite
		c = true
	}
	if old.PIDs != upd.PIDs {
		emp["pids"] = upd.PIDs
		c = true
	}
	if !c {
		return nil, ERR_NO_CHANGES
	}
	*a.handlers[upd.Name].stats = *upd
	return emp, nil
}

// format Форматирование статистики в строковые параметры.
func (a *App) format(s *container.StatsResponse) *entities.DockerStats {
	memUsage := a.calcMemUsage(s)
	netRx, netTx := a.calcNet(s.Networks)
	blockRead, blockWrite := a.calcBlock(s.BlkioStats.IoServiceBytesRecursive)
	return &entities.DockerStats{
		Id:         s.ID[:12],
		Name:       strings.TrimPrefix(s.Name, "/"),
		CPUPercent: fmt.Sprintf("%.2f%%", a.calcCPU(s)),
		MemUsage:   a.formatBytes(memUsage, 1024, 4, "iB"),
		MemLimit:   a.formatBytes(s.MemoryStats.Limit, 1024, 4, "iB"),
		MemPercent: fmt.Sprintf("%.2f%%", a.calcMemPercent(s.MemoryStats.Limit, memUsage)),
		NetRx:      a.formatBytes(netRx, 1000, 3, "B"),
		NetTx:      a.formatBytes(netTx, 1000, 3, "B"),
		BlockRead:  a.formatBytes(blockRead, 1000, 3, "B"),
		BlockWrite: a.formatBytes(blockWrite, 1000, 3, "B"),
		PIDs:       strconv.Itoa(int(s.PidsStats.Current)),
	}
}

// calcBlock Вычисления записи на диск.
func (a *App) calcBlock(io []container.BlkioStatEntry) (blockRead uint64, blockWrite uint64) {
	for _, bio := range io {
		switch strings.ToLower(bio.Op) {
		case "read":
			blockRead += bio.Value
		case "write":
			blockWrite += bio.Value
		}
	}
	return blockRead, blockWrite
}

// calcNet Вычисление сетевой активности.
// Суммируем трафик по всем сетевым интерфейсам контейнера.
func (a *App) calcNet(networks map[string]container.NetworkStats) (netRx uint64, netTx uint64) {
	for _, netStats := range networks {
		netRx += netStats.RxBytes
		netTx += netStats.TxBytes
	}
	return netRx, netTx
}

// calcCPU Вычисление процентажа использования процессора.
func (a *App) calcCPU(s *container.StatsResponse) (percent float64) {
	if s.CPUStats.CPUUsage.TotalUsage <= s.PreCPUStats.CPUUsage.TotalUsage ||
		s.CPUStats.SystemUsage <= s.PreCPUStats.SystemUsage {
		return 0.0
	}
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(s.CPUStats.SystemUsage - s.PreCPUStats.SystemUsage)
	onlineCPUs := float64(s.CPUStats.OnlineCPUs)
	if onlineCPUs == 0.0 {
		onlineCPUs = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		percent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	return percent
}

// calcMemUsage Вычисление использованой памяти.
func (a *App) calcMemUsage(s *container.StatsResponse) (usage uint64) {
	// 3. MEM USAGE / LIMIT и MEM %
	cache := s.MemoryStats.Stats["cache"]
	if cache == 0 {
		cache = s.MemoryStats.Stats["inactive_file"] // для cgroups v2
	}
	if s.MemoryStats.Usage > cache {
		usage = s.MemoryStats.Usage - cache
	}
	return usage
}

// calcMemPercent Вычисление процентного соотношения использованой памяти от лимита.
func (a *App) calcMemPercent(limit uint64, usage uint64) (percent float64) {
	if limit > 0 {
		percent = (float64(usage) / float64(limit)) * 100.0
	}
	return percent
}

// formatBytes Форматирование байт.
// Форматирует число number по указанному в unit шагу измерения.
// Обрезает результат до truncate символов.
// Также добавляет в конце суффикс suffix.
func (a *App) formatBytes(number, unit uint64, truncate int, suffix string) string {
	if number < unit {
		return fmt.Sprintf("%dB", number)
	}
	div, exp := uint64(unit), 0
	for n := number / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	r := float64(number) / float64(div)
	s := "KMGTPE"
	if unit == 1000 {
		s = "kMGTPE"
	}
	return fmt.Sprintf("%.*g%c%s", truncate, r, s[exp], suffix)
}
