package delivery

import (
	"encoding/json"
	"sync"
)

// ClientManager Управление клиентами.
type ClientManager struct {
	// Клиенты.
	clients map[*Client]struct{}
	mu      sync.Mutex
}

// NewClientManager Конструктор.
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[*Client]struct{}),
	}
}

// Register Регистрация.
func (cm *ClientManager) Register(c *Client) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.clients[c] = struct{}{}
}

// Broadcast Отправка сообщения всем клиентам.
func (cm *ClientManager) Broadcast(m json.RawMessage) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for c := range cm.clients {
		select {
		case c.send <- m:
		default:
			cm.removeClient(c)
		}
	}
}

// Unregister Отмена регистрации.
func (cm *ClientManager) Unregister(c *Client) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.removeClient(c)
}

// Close Закрытие менеджера клиентов.
func (cm *ClientManager) Close() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for c := range cm.clients {
		cm.removeClient(c)
	}
}

// removeCliuent Удаление клиента.
func (cm *ClientManager) removeClient(c *Client) {
	if _, o := cm.clients[c]; o {
		delete(cm.clients, c)
		c.Close()
	}
}
