package entities

// DockerStats Статистика Docker.
type DockerStats struct {
	Id         string `json:"container_id"`
	Name       string `json:"name"`
	CPUPercent string `json:"cpu_percent"`
	MemUsage   string `json:"mem_usage"`
	MemLimit   string `json:"mem_limit"`
	MemPercent string `json:"mem_percent"`
	NetRx      string `json:"net_rx"`
	NetTx      string `json:"net_tx"`
	BlockRead  string `json:"block_read"`
	BlockWrite string `json:"block_write"`
	PIDs       string `json:"pids"`
}
