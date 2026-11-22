package repository

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/skvdmt/skvdmt-docker-stats/internal/model"
)

// Docker Клиент.
type Docker struct {
	cli   *client.Client
	ctx   context.Context
	stats chan *container.StatsResponse
}

// NewDocker Конструктор.
func NewDocker(ctx context.Context) (*Docker, error) {
	model.Log.Info.Info("docker client creating")
	d := &Docker{
		ctx:   ctx,
		stats: make(chan *container.StatsResponse),
	}
	var err error
	if d.cli, err = client.New(client.FromEnv); err != nil {
		return nil, err
	}
	return d, nil
}

// DockerStats Канал для получения статистики Docker.
func (d *Docker) DockerStats() <-chan *container.StatsResponse {
	return d.stats
}

// DockerContainerNames Имена контейнеров Docker.
func (d *Docker) DockerContainerNames() (names []string, err error) {
	l, err := d.cli.ContainerList(d.ctx, client.ContainerListOptions{
		All: false,
	})
	if err != nil {
		return nil, err
	}
	for _, c := range l.Items {
		if len(c.Names) == 0 {
			continue
		}
		names = append(names, strings.TrimPrefix(c.Names[0], "/"))
	}
	return names, nil
}

// Создание обработчика статистики контейнера Docker с именем name.
func (d *Docker) DockerStatHandler(ctx context.Context, name string) {
	csr, err := d.cli.ContainerStats(ctx, name, client.ContainerStatsOptions{
		Stream: true,
	})
	if err != nil {
		model.MinorErrors <- err
		return
	}
	defer csr.Body.Close()
	dec := json.NewDecoder(csr.Body)
	for {
		var sr *container.StatsResponse
		if err := dec.Decode(&sr); err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return
			}
			model.MinorErrors <- err
			return
		}
		select {
		case d.stats <- sr:
		case <-ctx.Done():
			return
		}
	}
}
